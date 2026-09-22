// Package service provides system service management for hookly.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/kardianos/service"

	clicmd "hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/provision"
	"hooks.dx314.com/internal/relay"
)

const (
	serviceName        = "hookly"
	serviceDisplayName = "Hookly Webhook Relay"
	serviceDescription = "Webhook relay client for forwarding webhooks from edge to local services"
	shutdownTimeout    = 5 * time.Second
)

// Program implements service.Interface for the hookly relay.
type Program struct {
	cfg    *ServiceConfig
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start is called when the service is started.
// Must not block - start work in a goroutine.
func (p *Program) Start(s service.Service) error {
	slog.Info("service starting", "service", p.cfg.UnitName(), "config", p.cfg.ConfigPath)

	// Load hookly config
	hooklyCfg, err := config.LoadHooklyYAML(p.cfg.ConfigPath)
	if err != nil {
		return err
	}

	// The relay authenticates with the token saved by `hookly login`, for the
	// user the service runs as.
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}
	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials (%s): %w", credsMgr.Path(), err)
	}
	if creds == nil {
		return fmt.Errorf("not logged in: no credentials at %s - run 'hookly login' as the user the service runs as", credsMgr.Path())
	}
	hooklyCfg.Token = creds.APIToken

	// Named services share the host, so each needs its own hub ID or the edge
	// treats them as one hub and they keep replacing each other.
	if hooklyCfg.HubID == "" && p.cfg.Name != "" {
		hooklyCfg.HubID = config.HostHubID() + "-" + p.cfg.Name
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	// Create relay client
	client := relay.NewClient(hooklyCfg)

	// Start relay in goroutine (must not block)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if !provision.SyncForRelay(ctx, hooklyCfg, p.cfg.ConfigPath) {
			return
		}
		if err := client.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("relay error", "error", err)
		}
	}()

	slog.Info("service started",
		"edge_url", hooklyCfg.EdgeURL,
		"hub_id", hooklyCfg.GetHubID(),
		"endpoints", len(hooklyCfg.Endpoints),
		"proxies", len(hooklyCfg.Proxies),
	)

	return nil
}

// Stop is called when the service is stopped.
func (p *Program) Stop(s service.Service) error {
	slog.Info("service stopping")

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("service stopped gracefully")
	case <-time.After(shutdownTimeout):
		slog.Warn("service shutdown timed out")
	}

	return nil
}

// NewService creates a configured service.Service instance.
func NewService(cfg *ServiceConfig) (service.Service, error) {
	prg := &Program{cfg: cfg}

	options := make(service.KeyValue)
	options["KeepAlive"] = true
	options["RunAtLoad"] = true

	// For user services on macOS, set UserService option
	if cfg.UserService {
		options["UserService"] = true
	}

	// launchd writes stdout/stderr here (<unit>.out.log / <unit>.err.log)
	if dir := logDir(cfg.UserService); dir != "" {
		options["LogDirectory"] = dir
	}

	displayName := serviceDisplayName
	args := []string{"--service-mode", "--config", cfg.ConfigPath}
	if cfg.Name != "" {
		displayName += " (" + cfg.Name + ")"
		args = append(args, "--name", cfg.Name)
	}

	svcConfig := &service.Config{
		Name:        cfg.UnitName(),
		DisplayName: displayName,
		Description: serviceDescription,
		Arguments:   args,
		Option:      options,
	}

	// Set working directory if specified
	if cfg.WorkingDir != "" {
		svcConfig.WorkingDirectory = cfg.WorkingDir
	}

	return service.New(prg, svcConfig)
}

// RunServiceMode runs hookly in service mode (called by service manager).
// name is the instance name from --name ("" for the unnamed service).
func RunServiceMode(configPath, name string) error {
	// Setup logging for service mode
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := &ServiceConfig{
		Name:       name,
		ConfigPath: configPath,
	}

	svc, err := NewService(cfg)
	if err != nil {
		return err
	}

	return svc.Run()
}

// ControlService performs a control action on the service.
func ControlService(cfg *ServiceConfig, action string) error {
	svc, err := NewService(cfg)
	if err != nil {
		return err
	}

	switch action {
	case "install":
		if dir := logDir(cfg.UserService); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create log directory: %w", err)
			}
		}
		return svc.Install()
	case "uninstall":
		return svc.Uninstall()
	case "start":
		return svc.Start()
	case "stop":
		return svc.Stop()
	case "restart":
		return svc.Restart()
	default:
		return service.ErrNoServiceSystemDetected
	}
}

// GetServiceStatus returns the current service status.
func GetServiceStatus(cfg *ServiceConfig) (service.Status, error) {
	svc, err := NewService(cfg)
	if err != nil {
		return service.StatusUnknown, err
	}

	return svc.Status()
}

// StatusString returns a human-readable status string.
func StatusString(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}
