package events

import (
	"encoding/json"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{"play", Event{Type: "play", Source: "soundcloud", TrackID: "12"}, false},
		{"search", Event{Type: "search", Context: json.RawMessage(`{"q":"jazz"}`)}, false},
		{"missing track", Event{Type: "complete", Source: "soundcloud"}, true},
		{"unknown", Event{Type: "opened_player", Source: "soundcloud", TrackID: "12"}, true},
		{"invalid context", Event{Type: "play", Source: "soundcloud", TrackID: "12", Context: json.RawMessage(`{`)}, true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Validate(test.event); (got != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", got, test.wantErr)
			}
		})
	}
}
