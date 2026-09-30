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
		{"play", Event{Key: "event-1", Type: "play", Source: "soundcloud", TrackID: "12"}, false},
		{"search", Event{Key: "event-2", Type: "search", Context: json.RawMessage(`{"q":"jazz"}`)}, false},
		{"missing key", Event{Type: "play", Source: "soundcloud", TrackID: "12"}, true},
		{"missing track", Event{Key: "event-3", Type: "complete", Source: "soundcloud"}, true},
		{"unknown", Event{Key: "event-4", Type: "opened_player", Source: "soundcloud", TrackID: "12"}, true},
		{"invalid context", Event{Key: "event-5", Type: "play", Source: "soundcloud", TrackID: "12", Context: json.RawMessage(`{`)}, true},
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
