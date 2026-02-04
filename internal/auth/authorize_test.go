package auth

import (
	"context"
	"testing"
)

func TestAuthorizer_AllowedUsers(t *testing.T) {
	authorizer := NewAuthorizer(nil, "", []string{"alice", "bob"})

	tests := []struct {
		username string
		want     bool
	}{
		{"alice", true},
		{"Alice", true}, // Case insensitive
		{"ALICE", true},
		{"bob", true},
		{"charlie", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			got := authorizer.IsAuthorized(context.Background(), tt.username, "")
			if got != tt.want {
				t.Errorf("IsAuthorized(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestAuthorizer_NoRestrictions(t *testing.T) {
	authorizer := NewAuthorizer(nil, "", nil)

	// Without restrictions, all authenticated users should be allowed
	if !authorizer.IsAuthorized(context.Background(), "anyone", "") {
		t.Error("expected all users to be authorized when no restrictions")
	}
}

func TestAuthorizer_HasRestrictions(t *testing.T) {
	tests := []struct {
		name         string
		org          string
		allowedUsers []string
		want         bool
	}{
		{"no restrictions", "", nil, false},
		{"org only", "myorg", nil, true},
		{"users only", "", []string{"alice"}, true},
		{"both", "myorg", []string{"alice"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewAuthorizer(nil, tt.org, tt.allowedUsers)
			got := authorizer.HasRestrictions()
			if got != tt.want {
				t.Errorf("HasRestrictions() = %v, want %v", got, tt.want)
			}
		})
	}
}
