package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

// UniversalExtractHandler handles GET /api/v1/extract?url=...
// Extracts metadata and direct audio streaming link for any site supported by yt-dlp (YouTube, VK, Bandcamp, etc.).
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

	item, err := h.ytdlp.Extract(r.Context(), targetURL)
	if err != nil {
		if errors.Is(err, ytdlp.ErrUnsupportedURL) {
			Error(w, http.StatusBadRequest, "invalid or empty media URL")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, item)
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
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

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
			target = "https://www.youtube.com/watch?v=" + id
		}
	}

	if target == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' or 'id' is required")
		return
	}

	audioURL, err := h.ytdlp.ExtractAudioURL(r.Context(), target)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
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

	item, err := h.ytdlp.Extract(r.Context(), targetURL)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
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

	item, err := h.ytdlp.Extract(r.Context(), targetURL)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, item)
}
