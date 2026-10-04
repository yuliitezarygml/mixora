package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

func TestValidateYTDLPURLAllowsSupportedHTTPSProviders(t *testing.T) {
	t.Parallel()

	for _, targetURL := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://artist.bandcamp.com/track/a-public-track",
		"https://bandcamp.com/track/a-public-track",
		"https://vk.com/audio-1_2",
		"https://m.vk.com/audio-1_2",
		"https://vkvideo.ru/video-1_2",
	} {
		t.Run(targetURL, func(t *testing.T) {
			got, err := validateYTDLPURL(targetURL)
			if err != nil {
				t.Fatalf("validateYTDLPURL(%q) error = %v", targetURL, err)
			}
			if got != targetURL {
				t.Fatalf("validateYTDLPURL(%q) = %q, want unchanged URL", targetURL, got)
			}
		})
	}
}

func TestValidateYTDLPURLRejectsUnsafeOrUnsupportedTargets(t *testing.T) {
	t.Parallel()

	for _, targetURL := range []string{
		"http://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://user:password@www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com:444/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com:0443/watch?v=dQw4w9WgXcQ",
		"https://127.0.0.1/admin",
		"https://[::1]/admin",
		"https://localhost/admin",
		"https://app.localhost/admin",
		"https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
		"https://example.com/media",
	} {
		t.Run(targetURL, func(t *testing.T) {
			if _, err := validateYTDLPURL(targetURL); !isMediaURLPolicyError(err) {
				t.Fatalf("validateYTDLPURL(%q) error = %v, want policy error", targetURL, err)
			}
		})
	}
}

func TestValidateYouTubeURLRejectsOtherSupportedProviders(t *testing.T) {
	t.Parallel()

	if _, err := validateYouTubeURL("https://artist.bandcamp.com/track/a-public-track"); !isMediaURLPolicyError(err) {
		t.Fatalf("validateYouTubeURL() error = %v, want policy error", err)
	}
}

func TestYTDLPHandlersRejectUnsafeURLsBeforeRunningExtractor(t *testing.T) {
	// A non-empty but nonexistent binary reports as installed. If a rejected
	// target reaches yt-dlp, the handler returns a 500 from the failed exec;
	// the expected 400 proves the policy ran first.
	h := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("yt-dlp-must-not-run-in-policy-test")))

	tests := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{
			name:    "universal rejects metadata IP",
			path:    "/api/v1/extract?url=" + url.QueryEscape("https://169.254.169.254/latest/meta-data"),
			handler: h.UniversalExtractHandler,
		},
		{
			name:    "bandcamp route rejects unsupported host",
			path:    "/api/v1/bandcamp/resolve?url=" + url.QueryEscape("https://example.com/track"),
			handler: h.BandcampResolveHandler,
		},
		{
			name:    "VK route rejects credentials",
			path:    "/api/v1/vk/resolve?url=" + url.QueryEscape("https://user@vk.com/audio-1_2"),
			handler: h.VKResolveHandler,
		},
		{
			name:    "YouTube stream rejects URL smuggling",
			path:    "/api/v1/youtube/stream?url=" + url.QueryEscape("https://youtube.com@127.0.0.1/private"),
			handler: h.YouTubeStreamHandler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			tt.handler(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}

			var response Response
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Success || response.Error == "" {
				t.Fatalf("response = %#v, want a clear error", response)
			}
		})
	}
}
