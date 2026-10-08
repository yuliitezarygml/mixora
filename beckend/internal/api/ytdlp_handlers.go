package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
	ytdlpmodels "github.com/iulian/soundcloud-go/pkg/ytdlp/models"
)

const (
	maxExternalMediaBytes int64  = 512 << 20
	providerMediaChunk    uint64 = 1_000_000
)

// extractYTDLP keeps universal, Bandcamp, and VK resolution on one observer
// path. Their successful responses retain their provider-specific HTTP shape;
// only the compact converted track reaches the recommendation catalog.
func (h *Handler) extractYTDLP(ctx context.Context, targetURL string) (*ytdlpmodels.MediaItem, error) {
	targetURL, err := validateYTDLPURL(targetURL)
	if err != nil {
		return nil, err
	}

	item, err := h.ytdlp.Extract(ctx, targetURL)
	if err == nil && item != nil {
		h.observeYTDLPItem(ctx, *item)
	}
	return item, err
}

// UniversalExtractHandler handles GET /api/v1/extract?url=...
// Extracts metadata and direct audio streaming links for the supported
// YouTube, Bandcamp, and VK providers.
func (h *Handler) UniversalExtractHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() {
		Error(w, http.StatusServiceUnavailable, "yt-dlp is not installed or available on server")
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' is required")
		return
	}

	item, err := h.extractYTDLP(r.Context(), targetURL)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}

	JSON(w, http.StatusOK, item)
}

// ExternalStreamHandler resolves supported provider pages on the server and
// proxies their short-lived audio response from Mixora's own origin. Signed
// YouTube, VK, and Bandcamp CDN URLs are not durable browser API contracts and
// can reject a request made outside the extractor's network/header context.
func (h *Handler) ExternalStreamHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() || h.extractAudioSource == nil || h.mediaHTTP == nil {
		Error(w, http.StatusServiceUnavailable, "media extractor is temporarily unavailable")
		return
	}
	requestedRange, validRange := boundedSingleMediaRange(r.Header.Get("Range"))
	if !validRange {
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if h.mediaStreams != nil {
		select {
		case h.mediaStreams <- struct{}{}:
			defer func() { <-h.mediaStreams }()
		default:
			Error(w, http.StatusTooManyRequests, "too many media streams; try again shortly")
			return
		}
	}
	targetURL, err := validateYTDLPURL(r.URL.Query().Get("url"))
	if err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	source, err := h.extractAudioSource(r.Context(), targetURL)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}
	requestedStart, hasRequestedStart := mediaRangeStart(requestedRange)
	if source != nil && source.Size > 0 {
		var satisfiable bool
		requestedRange, requestedStart, satisfiable = clampMediaRangeToSize(requestedRange, source.Size)
		hasRequestedStart = true
		if !satisfiable {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", source.Size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}
	if source == nil {
		Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
		return
	}
	audioURL, err := validateExtractedAudioURL(source.URL)
	if err != nil {
		Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
		return
	}

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, audioURL, nil)
	if err != nil {
		Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
		return
	}
	copyExtractorMediaHeaders(upstream.Header, source.Headers)
	if upstream.Header.Get("Accept") == "" {
		upstream.Header.Set("Accept", "audio/*,*/*;q=0.8")
	}
	upstream.Header.Set("Accept-Encoding", "identity")
	if upstream.Header.Get("Referer") == "" {
		upstream.Header.Set("Referer", targetURL)
	}
	if upstream.Header.Get("User-Agent") == "" {
		upstream.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Mixora/1.0)")
	}
	if requestedRange != "" {
		upstream.Header.Set("Range", requestedRange)
	}

	response, err := h.mediaHTTP.Do(upstream)
	if err != nil {
		var networkError net.Error
		log.Printf("[WARN] media proxy upstream request failed timeout=%t", errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout())
		writeMediaProxyFailure(w, r, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if value := response.Header.Get("Content-Range"); value != "" {
			w.Header().Set("Content-Range", value)
		}
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		log.Printf("[WARN] media proxy upstream returned status=%d", response.StatusCode)
		Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
		return
	}
	if response.StatusCode == http.StatusPartialContent {
		if !contentRangeCoversStart(response.Header.Get("Content-Range"), requestedStart, hasRequestedStart) {
			log.Printf("[WARN] media proxy rejected upstream partial response with invalid content range")
			Error(w, http.StatusBadGateway, "media source returned an invalid range")
			return
		}
	} else if hasRequestedStart && requestedStart > 0 {
		// A full response starts at byte zero and cannot satisfy a deep seek.
		log.Printf("[WARN] media proxy rejected full response to a deep range request")
		Error(w, http.StatusBadGateway, "media source returned an invalid range")
		return
	}
	contentType := response.Header.Get("Content-Type")
	if !safeAudioContentType(contentType) {
		log.Printf("[WARN] media proxy rejected content type=%q", contentType)
		Error(w, http.StatusBadGateway, "media source returned unsupported content")
		return
	}
	if response.ContentLength > maxExternalMediaBytes {
		log.Printf("[WARN] media proxy rejected content length=%d", response.ContentLength)
		Error(w, http.StatusBadGateway, "media source is too large")
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, _ = copyExternalMedia(w, response.Body, maxExternalMediaBytes)
}

// clampMediaRangeToSize limits a normalized, provider-safe range to the known
// resource size. The returned start is the first byte the client requested.
func clampMediaRangeToSize(value string, size int64) (string, int64, bool) {
	if size <= 0 {
		return value, 0, true
	}
	if !strings.HasPrefix(value, "bytes=") {
		return value, 0, false
	}
	bounds := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(bounds) != 2 {
		return value, 0, false
	}
	if bounds[0] == "" {
		suffixLength, err := strconv.ParseInt(bounds[1], 10, 64)
		if err != nil || suffixLength <= 0 {
			return value, 0, false
		}
		if suffixLength >= size {
			return fmt.Sprintf("bytes=0-%d", size-1), 0, true
		}
		return value, size - suffixLength, true
	}
	start, err := strconv.ParseInt(bounds[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return value, start, false
	}
	end, err := strconv.ParseInt(bounds[1], 10, 64)
	if err != nil || end < start {
		return value, start, false
	}
	if end >= size {
		end = size - 1
	}
	return fmt.Sprintf("bytes=%d-%d", start, end), start, true
}

func mediaRangeStart(value string) (int64, bool) {
	if !strings.HasPrefix(value, "bytes=") {
		return 0, false
	}
	bounds := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(bounds) != 2 || bounds[0] == "" {
		return 0, false
	}
	start, err := strconv.ParseInt(bounds[0], 10, 64)
	return start, err == nil && start >= 0
}

func contentRangeCoversStart(value string, requestedStart int64, checkStart bool) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes ") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return false
	}
	bounds := strings.Split(parts[0], "-")
	if len(bounds) != 2 {
		return false
	}
	start, startErr := strconv.ParseInt(bounds[0], 10, 64)
	end, endErr := strconv.ParseInt(bounds[1], 10, 64)
	if startErr != nil || endErr != nil || start < 0 || end < start {
		return false
	}
	if parts[1] != "*" {
		total, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || total <= end {
			return false
		}
	}
	return !checkStart || start == requestedStart
}

func boundedSingleMediaRange(rawValue string) (string, bool) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return fmt.Sprintf("bytes=0-%d", providerMediaChunk-1), true
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return "", false
	}
	bounds := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(bounds) != 2 || (bounds[0] == "" && bounds[1] == "") {
		return "", false
	}
	var start, end uint64
	var err error
	if bounds[0] != "" {
		start, err = strconv.ParseUint(bounds[0], 10, 63)
		if err != nil {
			return "", false
		}
	}
	if bounds[1] != "" {
		end, err = strconv.ParseUint(bounds[1], 10, 63)
		if err != nil {
			return "", false
		}
	}
	if bounds[0] != "" && bounds[1] != "" && start > end {
		return "", false
	}
	if bounds[0] == "" {
		if end > providerMediaChunk {
			end = providerMediaChunk
		}
		return fmt.Sprintf("bytes=-%d", end), true
	}
	maximumEnd := start + providerMediaChunk - 1
	if maximumEnd < start {
		return "", false
	}
	if bounds[1] == "" || end > maximumEnd {
		end = maximumEnd
	}
	return fmt.Sprintf("bytes=%d-%d", start, end), true
}

func copyExtractorMediaHeaders(destination http.Header, source map[string]string) {
	allowed := map[string]bool{
		"Accept":          true,
		"Accept-Language": true,
		"Origin":          true,
		"Referer":         true,
		"Sec-Fetch-Mode":  true,
		"User-Agent":      true,
	}
	for name, value := range source {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if allowed[canonical] && value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n") {
			destination.Set(canonical, value)
		}
	}
}

func safeAudioContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return strings.HasPrefix(mediaType, "audio/") || mediaType == "application/octet-stream"
}

func writeMediaProxyFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
		Error(w, http.StatusGatewayTimeout, "media source timed out")
		return
	}
	Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
}

func copyExternalMedia(destination http.ResponseWriter, source io.Reader, maximum int64) (int64, error) {
	controller := http.NewResponseController(destination)
	buffer := make([]byte, 32*1024)
	var written int64
	for written <= maximum {
		remaining := maximum - written + 1
		chunk := buffer
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		read, readErr := source.Read(chunk)
		if read > 0 {
			if written+int64(read) > maximum {
				return written, fmt.Errorf("media response exceeded %d bytes", maximum)
			}
			_ = controller.SetWriteDeadline(time.Now().Add(mediaIdleIOTimeout))
			count, writeErr := destination.Write(chunk[:read])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
	return written, fmt.Errorf("media response exceeded %d bytes", maximum)
}

// YouTubeSearchHandler handles GET /api/v1/youtube/search?q=...&limit=...
func (h *Handler) YouTubeSearchHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() {
		Error(w, http.StatusServiceUnavailable, "yt-dlp is not installed or available on server")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		Error(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	limit := 5
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 25 {
			limit = l
		}
	}

	result, err := h.ytdlp.SearchYouTube(r.Context(), query, limit)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}
	h.observeYTDLPItems(r.Context(), result.Items)

	JSON(w, http.StatusOK, result)
}

// YouTubeStreamHandler handles GET /api/v1/youtube/stream?url=... or ?id=...
func (h *Handler) YouTubeStreamHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() {
		Error(w, http.StatusServiceUnavailable, "yt-dlp is not installed or available on server")
		return
	}

	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if target == "" {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id != "" {
			target = "https://www.youtube.com/watch?v=" + url.QueryEscape(id)
		}
	}

	if target == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' or 'id' is required")
		return
	}

	target, err := validateYouTubeURL(target)
	if err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	audioURL, err := h.ytdlp.ExtractAudioURL(r.Context(), target)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]string{
		"target":    target,
		"audio_url": audioURL,
	})
}

// BandcampResolveHandler handles GET /api/v1/bandcamp/resolve?url=...
func (h *Handler) BandcampResolveHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() {
		Error(w, http.StatusServiceUnavailable, "yt-dlp is not installed or available on server")
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' is required")
		return
	}

	targetURL, err := validateBandcampURL(targetURL)
	if err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.extractYTDLP(r.Context(), targetURL)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}

	JSON(w, http.StatusOK, item)
}

// VKResolveHandler handles GET /api/v1/vk/resolve?url=...
func (h *Handler) VKResolveHandler(w http.ResponseWriter, r *http.Request) {
	if h.ytdlp == nil || !h.ytdlp.IsInstalled() {
		Error(w, http.StatusServiceUnavailable, "yt-dlp is not installed or available on server")
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' is required")
		return
	}

	targetURL, err := validateVKURL(targetURL)
	if err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.extractYTDLP(r.Context(), targetURL)
	if err != nil {
		writeYTDLPFailure(w, err)
		return
	}

	JSON(w, http.StatusOK, item)
}

func isInvalidYTDLPRequest(err error) bool {
	return isMediaURLPolicyError(err) || errors.Is(err, ytdlp.ErrUnsupportedURL)
}

// writeYTDLPFailure is the public error boundary around the extractor. Its
// stderr belongs in server diagnostics, never in a browser response: it may
// include temporary media URLs or provider-specific operational details.
func writeYTDLPFailure(w http.ResponseWriter, err error) {
	if isInvalidYTDLPRequest(err) {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		Error(w, http.StatusGatewayTimeout, "media source timed out")
	case errors.Is(err, ytdlp.ErrBinaryNotFound):
		Error(w, http.StatusServiceUnavailable, "media extractor is temporarily unavailable")
	default:
		Error(w, http.StatusBadGateway, "media source is temporarily unavailable")
	}
}
