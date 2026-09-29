package httpapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizePlaybackState(t *testing.T) {
	clean, ok := normalizePlaybackState([]byte(`{"type":"state","playing":true,"position":3.5,"track":{"id":"1"},"queue":[{"id":"1"}],"extra":"drop"}`))
	if !ok || !bytes.Contains(clean, []byte(`"playing":true`)) || bytes.Contains(clean, []byte("extra")) {
		t.Fatalf("state was not normalized: %s %v", clean, ok)
	}
	if _, ok = normalizePlaybackState([]byte(`{"type":"other"}`)); ok {
		t.Fatal("unknown message accepted")
	}
	clean, ok = normalizePlaybackState([]byte(`{"type":"state","track":{"id":"1"},"queue":[` + strings.TrimRight(strings.Repeat(`{"id":"x"},`, 31), ",") + `]}`))
	if !ok {
		t.Fatal("long queue rejected")
	}
	var msg struct {
		Queue []json.RawMessage `json:"queue"`
	}
	if err := json.Unmarshal(clean, &msg); err != nil || len(msg.Queue) != 30 {
		t.Fatalf("queue not clipped: %s", clean)
	}
}
