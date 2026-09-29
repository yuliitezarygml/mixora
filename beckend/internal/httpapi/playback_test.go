package httpapi

import "testing"

func TestValidPlaybackState(t *testing.T) {
	t.Parallel()
	valid := []byte(`{"type":"state","playing":true,"position":12.5,"track":{"id":"1"},"queue":[]}`)
	if !validPlaybackState(valid) {
		t.Fatal("expected state to be valid")
	}
	for _, invalid := range [][]byte{
		[]byte(`{`),
		[]byte(`{"type":"ping","track":{"id":"1"}}`),
		[]byte(`{"type":"state"}`),
	} {
		if validPlaybackState(invalid) {
			t.Fatalf("expected %q to be invalid", invalid)
		}
	}
}
