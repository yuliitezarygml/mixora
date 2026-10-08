package ytdlp

import (
	"strings"
	"testing"
)

func TestParseAudioSourceKeepsProgressiveContractAndHeaders(t *testing.T) {
	source, err := parseAudioSource([]byte(`{
		"url":"https://cdn.example.test/audio.webm?signature=private",
		"protocol":"https",
		"ext":"webm",
		"filesize":4023523,
		"http_headers":{"User-Agent":"Extractor UA","Accept-Language":"en"}
	}`))
	if err != nil {
		t.Fatalf("parseAudioSource() error = %v", err)
	}
	if source.Protocol != "https" || source.Extension != "webm" || source.Size != 4023523 {
		t.Fatalf("source = %#v", source)
	}
	if source.Headers["User-Agent"] != "Extractor UA" {
		t.Fatalf("headers = %#v", source.Headers)
	}
}

func TestParseAudioSourceRejectsManifestProtocol(t *testing.T) {
	_, err := parseAudioSource([]byte(`{
		"url":"https://cdn.example.test/audio.m3u8",
		"protocol":"m3u8_native",
		"ext":"m4a"
	}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported audio protocol") {
		t.Fatalf("parseAudioSource() error = %v, want unsupported protocol", err)
	}
}
