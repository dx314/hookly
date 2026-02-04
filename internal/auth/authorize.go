package auth

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Authorizer handles user authorization checks.
type Authorizer struct {
	github       *GitHubClient
	org          string
	allowedUsers map[string]bool

	// Cache for org membership checks
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	member    bool
	expiresAt time.Time
}

const (
	cacheTTL = time.Hour
)

// NewAuthorizer creates a new authorizer.
// If org is set, users must be members of that organization.
// If allowedUsers is set, users must be in that list.
// If neither is set, all authenticated users are allowed.
func NewAuthorizer(github *GitHubClient, org string, allowedUsers []string) *Authorizer {
	allowed := make(map[string]bool)
	for _, u := range allowedUsers {
		u = strings.TrimSpace(u)
		if u != "" {
			allowed[strings.ToLower(u)] = true
		}
	}

	return &Authorizer{
		github:       github,
		org:          org,
		allowedUsers: allowed,
		cache:        make(map[string]cacheEntry),
	}
}

// IsAuthorized checks if the user is authorized to access the application.
// Returns true if authorized, false otherwise.
func (a *Authorizer) IsAuthorized(ctx context.Context, username string, accessToken string) bool {
	// Check allowed users list first (if configured)
	if len(a.allowedUsers) > 0 {
		if !a.allowedUsers[strings.ToLower(username)] {
			slog.Info("user not in allowed list", "username", username)
			return false
		}
	}

	// Check org membership (if configured)
	if a.org != "" {
		member, err := a.checkOrgMembership(ctx, username, accessToken)
		if err != nil {
			slog.Error("org membership check failed", "username", username, "org", a.org, "error", err)
			return false
		}
		if !member {
			slog.Info("user not member of org", "username", username, "org", a.org)
			return false
		}
	}

	// If neither is configured, allow all authenticated users
	return true
}

// checkOrgMembership checks org membership with caching.
func (a *Authorizer) checkOrgMembership(ctx context.Context, username, accessToken string) (bool, error) {
	cacheKey := username + ":" + a.org

	// Check cache
	a.mu.RLock()
	entry, ok := a.cache[cacheKey]
	a.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.member, nil
	}

	// Cache miss or expired, check GitHub
	member, err := a.github.CheckOrgMembership(ctx, accessToken, a.org)
	if err != nil {
		return false, err
	}

	// Update cache
	a.mu.Lock()
	a.cache[cacheKey] = cacheEntry{
		member:    member,
		expiresAt: time.Now().Add(cacheTTL),
	}
	a.mu.Unlock()

	return member, nil
}

// HasRestrictions returns true if any authorization restrictions are configured.
func (a *Authorizer) HasRestrictions() bool {
	return a.org != "" || len(a.allowedUsers) > 0
}
