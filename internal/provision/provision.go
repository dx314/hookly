// Package provision makes the edge match the endpoints declared in
// hookly.yaml: it creates missing endpoints and destinations, updates what
// differs, and fills the ids and webhook URLs the edge assigns back into the
// file, so hookly.yaml stays the one description of the setup.
package provision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	hooklyv1 "hooks.dx314.com/internal/api/hookly/v1"
	"hooks.dx314.com/internal/api/hookly/v1/hooklyv1connect"
	clicmd "hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/crypto"
)

// Options controls a Sync.
type Options struct {
	DryRun bool // Report what would change without changing anything
}

// Report is what a Sync did (or, for a dry run, would do).
type Report struct {
	Changes []string               // One line per change, for people
	Fields  []config.EndpointField // Values to fill into hookly.yaml
	Notes   []string               // Things left alone that may need a look
}

// permanentError marks a failure that retrying won't fix: a mistake in
// hookly.yaml or a request the edge refused.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func permanent(format string, args ...any) error {
	return &permanentError{fmt.Errorf(format, args...)}
}

// IsPermanent reports whether err won't go away by retrying. Anything else
// (edge unreachable, 5xx) is worth another try.
func IsPermanent(err error) bool {
	var pe *permanentError
	if errors.As(err, &pe) {
		return true
	}
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeAlreadyExists,
		connect.CodePermissionDenied, connect.CodeUnauthenticated, connect.CodeFailedPrecondition:
		return true
	}
	return false
}

// Sync makes the edge match cfg's endpoints. It fills in the ids of endpoints
// it creates or finds by name (in cfg, and as Report.Fields for the file).
// Settings hookly.yaml leaves out are not touched.
func Sync(ctx context.Context, edge hooklyv1connect.EdgeServiceClient, cfg *config.HooklyConfig, opts Options) (*Report, error) {
	s := &syncer{edge: edge, opts: opts, report: &Report{}}
	for i := range cfg.Endpoints {
		if err := s.endpoint(ctx, i, &cfg.Endpoints[i]); err != nil {
			return s.report, fmt.Errorf("endpoint %s: %w", cfg.Endpoints[i].Label(), err)
		}
	}
	return s.report, nil
}

type syncer struct {
	edge   hooklyv1connect.EdgeServiceClient
	opts   Options
	report *Report
	all    []*hooklyv1.Endpoint // The user's endpoints, listed once when needed
	label  string               // Name of the endpoint being synced, for messages
}

func (s *syncer) change(format string, args ...any) {
	prefix := ""
	if s.opts.DryRun {
		prefix = "would "
	}
	s.report.Changes = append(s.report.Changes, prefix+fmt.Sprintf(format, args...))
}

func (s *syncer) fill(i int, ep *config.EndpointConfig, key, value string) {
	switch key {
	case "id":
		if ep.ID == value {
			return
		}
		ep.ID = value
	case "url":
		if ep.URL == value {
			return
		}
		ep.URL = value
	}
	s.report.Fields = append(s.report.Fields, config.EndpointField{Endpoint: i, Key: key, Value: value})
}

func (s *syncer) endpoint(ctx context.Context, i int, ep *config.EndpointConfig) error {
	secret, err := ep.ResolvedSecret()
	if err != nil {
		return &permanentError{err}
	}

	remote, webhookURL, err := s.find(ctx, ep)
	if err != nil {
		return err
	}
	if remote == nil {
		return s.create(ctx, i, ep, secret)
	}

	s.label = ep.Name
	if s.label == "" {
		s.label = remote.Name
	}
	s.fill(i, ep, "id", remote.Id)
	if webhookURL != "" {
		s.fill(i, ep, "url", webhookURL)
	}
	if err := s.updateEndpoint(ctx, ep, remote, secret); err != nil {
		return err
	}
	return s.destinations(ctx, ep, remote)
}

// find returns the endpoint ep refers to, or nil when it doesn't exist yet:
// by id when the file has one, else by name.
func (s *syncer) find(ctx context.Context, ep *config.EndpointConfig) (*hooklyv1.Endpoint, string, error) {
	if ep.ID != "" {
		resp, err := s.edge.GetEndpoint(ctx, connect.NewRequest(&hooklyv1.GetEndpointRequest{Id: ep.ID}))
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, "", permanent("id %s doesn't exist on the edge (or isn't yours) - remove the id line to create a new endpoint", ep.ID)
		}
		if err != nil {
			return nil, "", err
		}
		return resp.Msg.Endpoint, resp.Msg.WebhookUrl, nil
	}

	if s.all == nil {
		all, err := s.listAll(ctx)
		if err != nil {
			return nil, "", err
		}
		s.all = all
	}
	var match *hooklyv1.Endpoint
	for _, e := range s.all {
		if e.Name != ep.Name {
			continue
		}
		if match != nil {
			return nil, "", permanent("several endpoints on the edge are named %q - add the id of the one to use", ep.Name)
		}
		match = e
	}
	if match == nil {
		return nil, "", nil
	}
	// The list has no webhook URLs
	resp, err := s.edge.GetEndpoint(ctx, connect.NewRequest(&hooklyv1.GetEndpointRequest{Id: match.Id}))
	if err != nil {
		return nil, "", err
	}
	return resp.Msg.Endpoint, resp.Msg.WebhookUrl, nil
}

func (s *syncer) listAll(ctx context.Context) ([]*hooklyv1.Endpoint, error) {
	var all []*hooklyv1.Endpoint
	token := ""
	for {
		resp, err := s.edge.ListEndpoints(ctx, connect.NewRequest(&hooklyv1.ListEndpointsRequest{
			Pagination: &hooklyv1.PaginationRequest{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Msg.Endpoints...)
		token = resp.Msg.GetPagination().GetNextPageToken()
		if token == "" {
			return all, nil
		}
	}
}

func (s *syncer) create(ctx context.Context, i int, ep *config.EndpointConfig, secret string) error {
	if ep.Name == "" {
		return permanent("has no name - an endpoint needs a name to be created")
	}
	req := &hooklyv1.CreateEndpointRequest{
		Name:               ep.Name,
		ProviderType:       providerType(ep.Provider),
		SignatureSecret:    secret,
		VerificationConfig: verificationConfig(ep.Verification),
	}
	for _, d := range ep.Destinations {
		req.Destinations = append(req.Destinations, &hooklyv1.DestinationInput{Name: d.Name, Url: d.URL, Enabled: d.Enabled})
	}
	if len(req.Destinations) == 0 {
		if ep.Destination == "" {
			return permanent("doesn't exist on the edge yet - list its destinations so hookly can create it")
		}
		req.DestinationUrl = ep.Destination
	}

	s.change("create endpoint %s (%s, %d destination(s))", ep.Name, providerName(req.ProviderType), max(len(req.Destinations), 1))
	if s.opts.DryRun {
		return nil
	}
	resp, err := s.edge.CreateEndpoint(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	s.fill(i, ep, "id", resp.Msg.Endpoint.Id)
	s.fill(i, ep, "url", resp.Msg.WebhookUrl)
	s.report.Notes = append(s.report.Notes,
		fmt.Sprintf("endpoint %s was created: point the provider at %s", ep.Name, resp.Msg.WebhookUrl))
	return nil
}

func (s *syncer) updateEndpoint(ctx context.Context, ep *config.EndpointConfig, remote *hooklyv1.Endpoint, secret string) error {
	req := &hooklyv1.UpdateEndpointRequest{Id: remote.Id}
	changed := false

	if ep.Name != "" && ep.Name != remote.Name {
		req.Name = proto.String(ep.Name)
		s.change("rename endpoint %s to %s", remote.Name, ep.Name)
		changed = true
	}
	wantProvider := providerType(ep.Provider)
	if ep.Provider != "" && wantProvider != remote.ProviderType {
		req.ProviderType = &wantProvider
		s.change("change endpoint %s provider from %s to %s", s.label, providerName(remote.ProviderType), ep.Provider)
		changed = true
	}
	if want := verificationConfig(ep.Verification); want != nil &&
		(req.ProviderType != nil || !proto.Equal(want, remote.VerificationConfig)) {
		req.VerificationConfig = want
		if req.ProviderType == nil {
			s.change("update endpoint %s verification", s.label)
		}
		changed = true
	}
	if ep.Muted != nil && *ep.Muted != remote.Muted {
		req.Muted = ep.Muted
		s.change("set endpoint %s muted=%t", s.label, *ep.Muted)
		changed = true
	}
	if len(ep.Destinations) == 0 && ep.Destination != "" && ep.Destination != remote.DestinationUrl {
		req.DestinationUrl = proto.String(ep.Destination)
		s.change("point endpoint %s's primary destination at %s", s.label, ep.Destination)
		changed = true
	}
	// The edge only reveals a fingerprint of the secret. An edge too old to
	// send one gets the secret every time, unreported as it's usually the same.
	if secret != "" {
		switch remote.SignatureSecretFingerprint {
		case crypto.SecretFingerprint(remote.Id, secret):
		case "":
			req.SignatureSecret = proto.String(secret)
		default:
			req.SignatureSecret = proto.String(secret)
			s.change("update endpoint %s secret", s.label)
			changed = true
		}
	}

	if s.opts.DryRun || (!changed && req.SignatureSecret == nil) {
		return nil
	}
	resp, err := s.edge.UpdateEndpoint(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if req.ProviderType != nil && resp.Msg.Endpoint.GetProviderType() != *req.ProviderType {
		s.report.Notes = append(s.report.Notes, fmt.Sprintf(
			"endpoint %s: the edge kept provider %s - it predates changing providers; upgrade the edge",
			s.label, providerName(resp.Msg.Endpoint.GetProviderType())))
	}
	return nil
}

func (s *syncer) destinations(ctx context.Context, ep *config.EndpointConfig, remote *hooklyv1.Endpoint) error {
	if len(ep.Destinations) == 0 {
		return nil // Legacy single destination is handled with the endpoint
	}
	byName := make(map[string]*hooklyv1.Destination, len(remote.Destinations))
	for _, d := range remote.Destinations {
		byName[d.Name] = d
	}

	for _, d := range ep.Destinations {
		enabled := d.Enabled == nil || *d.Enabled
		have, ok := byName[d.Name]
		delete(byName, d.Name)
		if !ok {
			s.change("add destination %s to %s (%s)", d.Name, s.label, d.URL)
			if s.opts.DryRun {
				continue
			}
			if _, err := s.edge.AddDestination(ctx, connect.NewRequest(&hooklyv1.AddDestinationRequest{
				EndpointId: remote.Id, Name: d.Name, Url: d.URL, Enabled: proto.Bool(enabled),
			})); err != nil {
				return fmt.Errorf("add destination %s: %w", d.Name, err)
			}
			continue
		}

		req := &hooklyv1.UpdateDestinationRequest{Id: have.Id}
		if have.Url != d.URL {
			req.Url = proto.String(d.URL)
			s.change("point destination %s of %s at %s", d.Name, s.label, d.URL)
		}
		if have.Enabled != enabled {
			req.Enabled = proto.Bool(enabled)
			s.change("set destination %s of %s enabled=%t", d.Name, s.label, enabled)
		}
		if s.opts.DryRun || (req.Url == nil && req.Enabled == nil) {
			continue
		}
		if _, err := s.edge.UpdateDestination(ctx, connect.NewRequest(req)); err != nil {
			return fmt.Errorf("update destination %s: %w", d.Name, err)
		}
	}

	// On the edge but not in the file, in position order
	for _, d := range remote.Destinations {
		if _, extra := byName[d.Name]; !extra {
			continue
		}
		if !ep.Prune {
			s.report.Notes = append(s.report.Notes, fmt.Sprintf(
				"endpoint %s has destination %s on the edge that hookly.yaml doesn't list (left alone; set prune: true to remove it)",
				s.label, d.Name))
			continue
		}
		s.change("remove destination %s from %s", d.Name, s.label)
		if s.opts.DryRun {
			continue
		}
		if _, err := s.edge.RemoveDestination(ctx, connect.NewRequest(&hooklyv1.RemoveDestinationRequest{Id: d.Id})); err != nil {
			return fmt.Errorf("remove destination %s: %w", d.Name, err)
		}
	}
	return nil
}

// Apply syncs cfg to the edge with the given login and writes the filled-in
// values back to the hookly.yaml at path. A failed write-back is only logged:
// the edge is already right and the next run finds the endpoints by name.
func Apply(ctx context.Context, cfg *config.HooklyConfig, path string, opts Options) (*Report, error) {
	if len(cfg.Endpoints) == 0 {
		return &Report{}, nil
	}
	return ApplyWith(ctx, clicmd.NewClient(cfg.EdgeURL, cfg.Token).Edge, cfg, path, opts)
}

// ApplyWith is Apply with a given edge client.
func ApplyWith(ctx context.Context, edge hooklyv1connect.EdgeServiceClient, cfg *config.HooklyConfig, path string, opts Options) (*Report, error) {
	report, err := Sync(ctx, edge, cfg, opts)
	if len(report.Fields) > 0 && !opts.DryRun {
		if werr := config.SetEndpointFields(path, report.Fields); werr != nil {
			slog.Warn("could not record ids in hookly.yaml", "path", path, "error", werr)
			for _, f := range report.Fields {
				report.Notes = append(report.Notes, fmt.Sprintf("add to endpoint %d in %s: %s: %q", f.Endpoint, path, f.Key, f.Value))
			}
		}
	}
	return report, err
}

// ApplyWithRetry is Apply for the relay: it retries until the edge is
// reachable (a relay started at boot may come up before the network) and
// only gives up on permanent errors.
func ApplyWithRetry(ctx context.Context, cfg *config.HooklyConfig, path string) error {
	backoff := time.Second
	for {
		report, err := Apply(ctx, cfg, path, Options{})
		if report != nil {
			for _, c := range report.Changes {
				slog.Info("synced hookly.yaml to edge", "change", c)
			}
			for _, n := range report.Notes {
				slog.Warn(n)
			}
		}
		if err == nil {
			return nil
		}
		if IsPermanent(err) || ctx.Err() != nil {
			return err
		}
		slog.Warn("syncing hookly.yaml to the edge failed, will retry", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

// SyncForRelay syncs hookly.yaml before the relay connects, retrying until
// the edge is reachable. It reports whether the relay can start: after a
// permanent failure (a mistake in hookly.yaml, say) it still runs as long as
// every endpoint has an id, so a typo never stops webhooks that were flowing.
func SyncForRelay(ctx context.Context, cfg *config.HooklyConfig, path string) bool {
	err := ApplyWithRetry(ctx, cfg, path)
	if err == nil {
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	for _, ep := range cfg.Endpoints {
		if ep.ID == "" {
			slog.Error("could not sync hookly.yaml to the edge; not starting the relay", "error", err)
			return false
		}
	}
	slog.Error("could not sync hookly.yaml to the edge; relaying with the edge's current setup", "error", err)
	return true
}

func providerType(name string) hooklyv1.ProviderType {
	switch name {
	case "stripe":
		return hooklyv1.ProviderType_PROVIDER_TYPE_STRIPE
	case "github":
		return hooklyv1.ProviderType_PROVIDER_TYPE_GITHUB
	case "telegram":
		return hooklyv1.ProviderType_PROVIDER_TYPE_TELEGRAM
	case "custom":
		return hooklyv1.ProviderType_PROVIDER_TYPE_CUSTOM
	}
	return hooklyv1.ProviderType_PROVIDER_TYPE_GENERIC
}

func providerName(pt hooklyv1.ProviderType) string {
	switch pt {
	case hooklyv1.ProviderType_PROVIDER_TYPE_STRIPE:
		return "stripe"
	case hooklyv1.ProviderType_PROVIDER_TYPE_GITHUB:
		return "github"
	case hooklyv1.ProviderType_PROVIDER_TYPE_TELEGRAM:
		return "telegram"
	case hooklyv1.ProviderType_PROVIDER_TYPE_CUSTOM:
		return "custom"
	}
	return "generic"
}

func verificationConfig(v *config.VerificationConfig) *hooklyv1.VerificationConfig {
	if v == nil {
		return nil
	}
	methods := map[string]hooklyv1.VerificationMethod{
		"static":           hooklyv1.VerificationMethod_VERIFICATION_METHOD_STATIC,
		"hmac_sha256":      hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA256,
		"hmac_sha1":        hooklyv1.VerificationMethod_VERIFICATION_METHOD_HMAC_SHA1,
		"timestamped_hmac": hooklyv1.VerificationMethod_VERIFICATION_METHOD_TIMESTAMPED_HMAC,
	}
	return &hooklyv1.VerificationConfig{
		Method:             methods[v.Method],
		SignatureHeader:    v.SignatureHeader,
		SignaturePrefix:    v.SignaturePrefix,
		TimestampHeader:    v.TimestampHeader,
		TimestampTolerance: v.TimestampTolerance,
	}
}
