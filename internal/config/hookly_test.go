package config

import "testing"

func TestGetDestination(t *testing.T) {
	cfg := &HooklyConfig{Endpoints: []EndpointConfig{
		{ID: "legacy", Destination: "http://local/primary"},
		{ID: "named", Destinations: DestinationList{{Name: "schoolboy", URL: "http://local/schoolboy"}}},
		{ID: "both", Destination: "http://local/primary", Destinations: DestinationList{{Name: "schoolboy", URL: "http://local/schoolboy"}}},
		{ID: "none"},
	}}

	tests := []struct {
		name     string
		endpoint string
		destName string
		primary  bool
		want     string
	}{
		{"legacy override applies to primary", "legacy", "otto", true, "http://local/primary"},
		{"legacy override never captures a second destination", "legacy", "schoolboy", false, "http://edge/default"},
		{"legacy override applies to envelopes from an old edge", "legacy", "", false, "http://local/primary"},
		{"named override", "named", "schoolboy", false, "http://local/schoolboy"},
		{"named override missing falls back to edge url", "named", "otto", true, "http://edge/default"},
		{"named override wins over legacy for primary", "both", "schoolboy", true, "http://local/schoolboy"},
		{"legacy still applies to unnamed primary", "both", "otto", true, "http://local/primary"},
		{"no override", "none", "otto", true, "http://edge/default"},
		{"unknown endpoint", "other", "otto", true, "http://edge/default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.GetDestination(tt.endpoint, tt.destName, tt.primary, "http://edge/default"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
