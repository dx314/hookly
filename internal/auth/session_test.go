package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}
	if state1 == "" {
		t.Error("GenerateState() returned empty string")
	}

	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}
	if state1 == state2 {
		t.Error("GenerateState() should generate unique states")
	}
}

func TestValidateState(t *testing.T) {
	tests := []struct {
		name        string
		cookieVal   string
		queryState  string
		wantValid   bool
	}{
		{
			name:       "matching state",
			cookieVal:  "abc123",
			queryState: "abc123",
			wantValid:  true,
		},
		{
			name:       "mismatched state",
			cookieVal:  "abc123",
			queryState: "xyz789",
			wantValid:  false,
		},
		{
			name:       "empty state",
			cookieVal:  "",
			queryState: "",
			wantValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/callback?state="+tt.queryState, nil)
			req.AddCookie(&http.Cookie{Name: StateCookieName, Value: tt.cookieVal})

			got := ValidateState(req, tt.queryState)
			if got != tt.wantValid {
				t.Errorf("ValidateState() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestValidateState_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/callback?state=abc123", nil)
	got := ValidateState(req, "abc123")
	if got != false {
		t.Error("ValidateState() should return false when no cookie")
	}
}
