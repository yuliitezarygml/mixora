package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

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
