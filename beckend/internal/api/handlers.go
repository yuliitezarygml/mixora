package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/soundcloud"
	"github.com/iulian/soundcloud-go/pkg/spotify"
	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

// Handler holds the soundcloud, spotify and ytdlp clients and handlers.
type Handler struct {
	client  *soundcloud.Client
	spotify *spotify.Client
	ytdlp   *ytdlp.Client
}

// NewHandler creates a new unified API Handler instance.
func NewHandler(client *soundcloud.Client, spClient *spotify.Client, ytClient *ytdlp.Client) *Handler {
	return &Handler{
		client:  client,
		spotify: spClient,
		ytdlp:   ytClient,
	}
}

// HealthHandler returns API health status for all services.
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	scID := ""
	if h.client != nil {
		scID = h.client.ClientID()
	}

	spConnectOk := false
	if h.spotify != nil && h.spotify.Connect() != nil {
		spConnectOk = h.spotify.Connect().IsAvailable(r.Context())
	}

	spBinaryInstalled := false
	binType := ""
	if h.spotify != nil && h.spotify.Supervisor() != nil {
		spBinaryInstalled = h.spotify.Supervisor().IsInstalled()
		_, binType = h.spotify.Supervisor().BinaryInfo()
	}

	ytdlpInstalled := false
	ytdlpVersion := ""
	ytdlpPath := ""
	if h.ytdlp != nil && h.ytdlp.IsInstalled() {
		ytdlpInstalled = true
		ytdlpPath = h.ytdlp.BinaryPath()
		ytdlpVersion, _ = h.ytdlp.Version(r.Context())
	}

	JSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"soundcloud": map[string]any{
			"status":    "ready",
			"client_id": scID,
		},
		"spotify": map[string]any{
			"status":                   "ready",
			"connect_daemon_reachable": spConnectOk,
			"connect_binary_installed": spBinaryInstalled,
			"connect_binary_type":      binType,
		},
		"ytdlp": map[string]any{
			"status":    "ready",
			"installed": ytdlpInstalled,
			"version":   ytdlpVersion,
			"path":      ytdlpPath,
		},
	})
}

// ResolveHandler handles GET /api/v1/resolve?url=...
func (h *Handler) ResolveHandler(w http.ResponseWriter, r *http.Request) {
	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' is required")
		return
	}

	raw, err := h.client.Resolve(r.Context(), targetURL)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "resource not found on SoundCloud")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// GetTrackHandler handles GET /api/v1/tracks/{id}
func (h *Handler) GetTrackHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/tracks/")
	idStr = strings.TrimSuffix(idStr, "/stream")
	idStr = strings.TrimSuffix(idStr, "/related")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	track, err := h.client.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "track not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, track)
}

// GetStreamHandler handles GET /api/v1/tracks/{id}/stream
// Returns the resolved direct stream URL for audio playback.
func (h *Handler) GetStreamHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid stream route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	track, err := h.client.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "track not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	streamURL, format, err := h.client.GetBestStreamURL(r.Context(), track)
	if err != nil {
		Error(w, http.StatusNotFound, "no playable stream found: "+err.Error())
		return
	}

	allFormats := h.client.GetAllStreamFormats(r.Context(), track)

	JSON(w, http.StatusOK, map[string]any{
		"track_id":    track.ID,
		"title":       track.Title,
		"format":      format,
		"stream_url":  streamURL,
		"all_formats": allFormats,
		"duration":    track.Duration,
	})
}

// GetLyricsHandler handles GET /api/v1/tracks/{id}/lyrics
func (h *Handler) GetLyricsHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid lyrics route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	track, err := h.client.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "track not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	lyrics, err := h.client.GetTrackLyrics(r.Context(), track)
	if err != nil {
		Error(w, http.StatusNotFound, err.Error())
		return
	}

	JSON(w, http.StatusOK, lyrics)
}

// GetRelatedHandler handles GET /api/v1/tracks/{id}/related
func (h *Handler) GetRelatedHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid related route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	res, err := h.client.GetRelatedTracks(r.Context(), id, soundcloud.SearchOptions{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, res)
}

// GetWaveformHandler handles GET /api/v1/tracks/{id}/waveform
func (h *Handler) GetWaveformHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid waveform route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	track, err := h.client.GetTrack(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "track not found")
		return
	}

	wf, err := h.client.GetTrackWaveform(r.Context(), track)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, wf)
}

// GetOriginalDownloadHandler handles GET /api/v1/tracks/{id}/download
func (h *Handler) GetOriginalDownloadHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid download route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid track id")
		return
	}

	downloadURL, err := h.client.GetOriginalDownloadURL(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"track_id":     id,
		"download_url": downloadURL,
	})
}

// GetTrendingHandler handles GET /api/v1/charts/trending?genre=all-music&limit=20
func (h *Handler) GetTrendingHandler(w http.ResponseWriter, r *http.Request) {
	genre := r.URL.Query().Get("genre")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	charts, err := h.client.GetTrending(r.Context(), genre, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, charts)
}

// SearchHandler handles GET /api/v1/search?q=...&type=tracks|users|playlists|all
func (h *Handler) SearchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		Error(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	searchType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	opts := soundcloud.SearchOptions{Limit: limit, Offset: offset}

	switch searchType {
	case "tracks":
		res, err := h.client.SearchTracks(r.Context(), q, opts)
		if err != nil {
			Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		JSON(w, http.StatusOK, res)
	case "users":
		res, err := h.client.SearchUsers(r.Context(), q, opts)
		if err != nil {
			Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		JSON(w, http.StatusOK, res)
	case "playlists":
		res, err := h.client.SearchPlaylists(r.Context(), q, opts)
		if err != nil {
			Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		JSON(w, http.StatusOK, res)
	default:
		res, err := h.client.Search(r.Context(), q, opts)
		if err != nil {
			Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		JSON(w, http.StatusOK, res)
	}
}

// GetUserHandler handles GET /api/v1/users/{id}
func (h *Handler) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	idStr = strings.TrimSuffix(idStr, "/tracks")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := h.client.GetUser(r.Context(), id)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "user not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, user)
}

// GetUserTracksHandler handles GET /api/v1/users/{id}/tracks
func (h *Handler) GetUserTracksHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		Error(w, http.StatusBadRequest, "invalid user tracks route format")
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tracks, err := h.client.GetUserTracks(r.Context(), id, soundcloud.SearchOptions{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, tracks)
}

// GetPlaylistHandler handles GET /api/v1/playlists/{id}
func (h *Handler) GetPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/playlists/")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid playlist id")
		return
	}

	pl, err := h.client.GetPlaylist(r.Context(), id)
	if err != nil {
		if errors.Is(err, soundcloud.ErrNotFound) {
			Error(w, http.StatusNotFound, "playlist not found")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, pl)
}
