package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

type mediaRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mediaRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestExternalStreamProxiesProviderAudioWithRange(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{
			URL:      "https://cdn.example.test/audio.webm?signature=private",
			Protocol: "https",
			Headers:  map[string]string{"User-Agent": "yt-dlp exact UA", "Authorization": "must-not-forward"},
			Size:     10,
		}, nil
	}
	var upstreamRange, upstreamUserAgent, upstreamAuthorization string
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamRange = request.Header.Get("Range")
		upstreamUserAgent = request.Header.Get("User-Agent")
		upstreamAuthorization = request.Header.Get("Authorization")
		return &http.Response{
			StatusCode:    http.StatusPartialContent,
			Header:        http.Header{"Content-Type": {"audio/webm"}, "Content-Range": {"bytes 0-4/10"}, "Accept-Ranges": {"bytes"}},
			Body:          io.NopCloser(strings.NewReader("audio")),
			ContentLength: 5,
			Request:       request,
		}, nil
	})}

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"),
		nil,
	)
	request.Header.Set("Range", "bytes=0-4")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusPartialContent, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != "audio" {
		t.Fatalf("body = %q, want audio", got)
	}
	if upstreamRange != "bytes=0-4" {
		t.Fatalf("upstream Range = %q, want bytes=0-4", upstreamRange)
	}
	if upstreamUserAgent != "yt-dlp exact UA" || upstreamAuthorization != "" {
		t.Fatalf("upstream headers User-Agent=%q Authorization=%q", upstreamUserAgent, upstreamAuthorization)
	}
	for header, want := range map[string]string{
		"Content-Type":  "audio/webm",
		"Content-Range": "bytes 0-4/10",
		"Accept-Ranges": "bytes",
		"Cache-Control": "private, no-store",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestExternalStreamRejectsUnsafeExtractedTarget(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "http://127.0.0.1/private", Protocol: "http"}, nil
	}
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe extracted URL reached HTTP client")
		return nil, nil
	})}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/media/stream?url="+url.QueryEscape("https://artist.bandcamp.com/track/public-track"),
		nil,
	)

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "127.0.0.1") {
		t.Fatalf("response exposed unsafe upstream: %s", recorder.Body.String())
	}
}

func TestExternalStreamRejectsInvalidRangeBeforeExtraction(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	extracted := false
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		extracted = true
		return nil, fmt.Errorf("must not run")
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"),
		nil,
	)
	request.Header.Set("Range", "bytes=-")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusRequestedRangeNotSatisfiable || extracted {
		t.Fatalf("status=%d extracted=%t", recorder.Code, extracted)
	}
}

func TestBoundedSingleMediaRangeAddsProviderSafeChunks(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", "bytes=0-999999"},
		{"bytes=0-", "bytes=0-999999"},
		{"bytes=1000000-", "bytes=1000000-1999999"},
		{"bytes=5-9", "bytes=5-9"},
		{"bytes=-2000000", "bytes=-1000000"},
	} {
		got, ok := boundedSingleMediaRange(test.input)
		if !ok || got != test.want {
			t.Errorf("boundedSingleMediaRange(%q) = %q/%t, want %q/true", test.input, got, ok, test.want)
		}
	}
}

func TestExternalStreamClampsRangeToKnownAudioSize(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "https://cdn.example.test/audio.webm", Protocol: "https", Size: 123}, nil
	}
	var upstreamRange string
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamRange = request.Header.Get("Range")
		return &http.Response{
			StatusCode:    http.StatusPartialContent,
			Header:        http.Header{"Content-Type": {"audio/webm"}, "Content-Range": {"bytes 100-122/123"}},
			Body:          io.NopCloser(strings.NewReader("audio")),
			ContentLength: 5,
			Request:       request,
		}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"), nil)
	request.Header.Set("Range", "bytes=100-")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if upstreamRange != "bytes=100-122" {
		t.Fatalf("upstream Range = %q, want bytes=100-122", upstreamRange)
	}
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusPartialContent, recorder.Body.String())
	}
}

func TestExternalStreamReturns416ForRangeAtKnownEOF(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "https://cdn.example.test/audio.webm", Protocol: "https", Size: 123}, nil
	}
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("range at EOF must not reach upstream")
		return nil, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"), nil)
	request.Header.Set("Range", "bytes=123-")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusRequestedRangeNotSatisfiable || recorder.Header().Get("Content-Range") != "bytes */123" {
		t.Fatalf("status=%d Content-Range=%q", recorder.Code, recorder.Header().Get("Content-Range"))
	}
}

func TestExternalStreamRejectsUpstreamRangeThatMissesRequestedStart(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "https://cdn.example.test/audio.webm", Protocol: "https", Size: 1000}, nil
	}
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Type": {"audio/webm"}, "Content-Range": {"bytes 0-99/1000"}},
			Body:       io.NopCloser(strings.NewReader("wrong chunk")),
			Request:    request,
		}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"), nil)
	request.Header.Set("Range", "bytes=500-599")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "wrong chunk") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestExternalStreamRejectsUpstreamRangeThatStartsBeforeRequestedByte(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "https://cdn.example.test/audio.webm", Protocol: "https", Size: 1000}, nil
	}
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Type": {"audio/webm"}, "Content-Range": {"bytes 0-599/1000"}},
			Body:       io.NopCloser(strings.NewReader("overlapping wrong chunk")),
			Request:    request,
		}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/media/stream?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"), nil)
	request.Header.Set("Range", "bytes=500-599")
	recorder := httptest.NewRecorder()

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "overlapping wrong chunk") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestExternalStreamRejectsNonAudioResponse(t *testing.T) {
	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("test-ytdlp")))
	handler.extractAudioSource = func(context.Context, string) (*ytdlp.AudioSource, error) {
		return &ytdlp.AudioSource{URL: "https://cdn.example.test/login", Protocol: "https"}, nil
	}
	handler.mediaHTTP = &http.Client{Transport: mediaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader("<script>no</script>")),
			Request:    request,
		}, nil
	})}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/media/stream?url="+url.QueryEscape("https://artist.bandcamp.com/track/public-track"),
		nil,
	)

	handler.ExternalStreamHandler(recorder, request)

	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "script") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestWriteYTDLPFailureUsesSafeRetryableResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "extractor stderr is hidden",
			err:         fmt.Errorf("%w: ERROR: token=secret https://cdn.example/signed", ytdlp.ErrExtractionFailed),
			wantStatus:  http.StatusBadGateway,
			wantMessage: "media source is temporarily unavailable",
		},
		{
			name:        "deadline maps to retryable gateway timeout",
			err:         fmt.Errorf("%w: %w", ytdlp.ErrExtractionFailed, context.DeadlineExceeded),
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "media source timed out",
		},
		{
			name:        "missing binary maps to service unavailable",
			err:         ytdlp.ErrBinaryNotFound,
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "media extractor is temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeYTDLPFailure(recorder, tt.err)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var response Response
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Error != tt.wantMessage {
				t.Fatalf("message = %q, want %q", response.Error, tt.wantMessage)
			}
			if strings.Contains(response.Error, "secret") || strings.Contains(response.Error, "cdn.example") {
				t.Fatalf("response exposed extractor diagnostics: %q", response.Error)
			}
		})
	}
}

func TestUniversalExtractDoesNotExposeFailedProcessDetails(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil, ytdlp.New(ytdlp.WithBinaryPath("yt-dlp-does-not-exist-for-test")))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/extract?url="+url.QueryEscape("https://www.youtube.com/watch?v=dQw4w9WgXcQ"),
		nil,
	)

	handler.UniversalExtractHandler(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	var response Response
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "media source is temporarily unavailable" {
		t.Fatalf("message = %q", response.Error)
	}
}
