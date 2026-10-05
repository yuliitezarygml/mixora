package recommendation

import "testing"

func TestCanonicalCatalogLookupKeysNormalizesLegacySoundCloudRefs(t *testing.T) {
	keys, legacyIDs := canonicalCatalogLookupKeys([]string{
		"soundcloud:soundcloud:tracks:42",
		"spotify:track-1",
		"soundcloud:42",
		"not-a-track-key",
	})
	wantKeys := []string{"soundcloud:42", "spotify:track-1", "not-a-track-key"}
	wantLegacyIDs := []string{"42"}
	if len(keys) != len(wantKeys) {
		t.Fatalf("keys = %#v, want %#v", keys, wantKeys)
	}
	for index := range wantKeys {
		if keys[index] != wantKeys[index] {
			t.Fatalf("keys[%d] = %q, want %q", index, keys[index], wantKeys[index])
		}
	}
	if len(legacyIDs) != len(wantLegacyIDs) || legacyIDs[0] != wantLegacyIDs[0] {
		t.Fatalf("legacySoundCloudIDs = %#v, want %#v", legacyIDs, wantLegacyIDs)
	}
}
