package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iulian/soundcloud-go/pkg/spotify"
)

// SpotifyResolveHandler handles GET /api/v1/spotify/resolve?url=...
func (h *Handler) SpotifyResolveHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if targetURL == "" {
		Error(w, http.StatusBadRequest, "query parameter 'url' is required")
		return
	}

	res, err := h.spotify.Resolve(r.Context(), targetURL)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "resource not found on Spotify")
			return
		}
		if errors.Is(err, spotify.ErrInvalidURL) {
			Error(w, http.StatusBadRequest, "invalid Spotify URL or URI")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, res)
}

// SpotifyTrackHandler handles GET /api/v1/spotify/tracks/{id}
func (h *Handler) SpotifyTrackHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/tracks/")
	id = strings.TrimSuffix(id, "/stream")
	id = strings.TrimSuffix(id, "/lyrics")

	if id == "" {
		Error(w, http.StatusBadRequest, "track ID is required")
		return
	}

	track, err := h.spotify.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "track not found on Spotify")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, track)
}

// SpotifyStreamHandler handles GET /api/v1/spotify/tracks/{id}/stream
func (h *Handler) SpotifyStreamHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/tracks/")
	id = strings.TrimSuffix(id, "/stream")

	streamInfo, err := h.spotify.GetStream(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "track not found on Spotify")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, streamInfo)
}

// SpotifyLyricsHandler handles GET /api/v1/spotify/tracks/{id}/lyrics
func (h *Handler) SpotifyLyricsHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/tracks/")
	id = strings.TrimSuffix(id, "/lyrics")

	lyrics, err := h.spotify.GetTrackLyrics(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNoLyrics) {
			Error(w, http.StatusNotFound, "no lyrics available for this track")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, lyrics)
}

// SpotifyAlbumHandler handles GET /api/v1/spotify/albums/{id}
func (h *Handler) SpotifyAlbumHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/albums/")
	if id == "" {
		Error(w, http.StatusBadRequest, "album ID is required")
		return
	}

	album, err := h.spotify.GetAlbum(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "album not found on Spotify")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, album)
}

// SpotifyArtistHandler handles GET /api/v1/spotify/artists/{id}
func (h *Handler) SpotifyArtistHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/artists/")
	if id == "" {
		Error(w, http.StatusBadRequest, "artist ID is required")
		return
	}

	artist, err := h.spotify.GetArtist(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "artist not found on Spotify")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, artist)
}

// SpotifyPlaylistHandler handles GET /api/v1/spotify/playlists/{id}
func (h *Handler) SpotifyPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/playlists/")
	if id == "" {
		Error(w, http.StatusBadRequest, "playlist ID is required")
		return
	}

	playlist, err := h.spotify.GetPlaylist(r.Context(), id)
	if err != nil {
		if errors.Is(err, spotify.ErrNotFound) {
			Error(w, http.StatusNotFound, "playlist not found on Spotify")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, playlist)
}

// SpotifySearchHandler handles GET /api/v1/spotify/search?q=...&type=...&limit=...
func (h *Handler) SpotifySearchHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify service not initialized")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		Error(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	sType := spotify.ResourceType(strings.TrimSpace(r.URL.Query().Get("type")))
	if sType == "" {
		sType = spotify.TypeTrack
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	res, err := h.spotify.Search(r.Context(), query, sType, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, res)
}

// SpotifyConnectStatusHandler handles GET /api/v1/spotify/connect/status
func (h *Handler) SpotifyConnectStatusHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil || h.spotify.Connect() == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify Connect not configured")
		return
	}

	status, err := h.spotify.Connect().Status(r.Context())
	if err != nil {
		if errors.Is(err, spotify.ErrConnectUnavailable) {
			Error(w, http.StatusServiceUnavailable, "Spotify Connect daemon not running (start go-librespot or librespot)")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, status)
}

// SpotifyConnectActionHandler handles POST /api/v1/spotify/connect/player/{action}
func (h *Handler) SpotifyConnectActionHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil || h.spotify.Connect() == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify Connect not configured")
		return
	}

	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST")
		return
	}

	action := strings.TrimPrefix(r.URL.Path, "/api/v1/spotify/connect/player/")
	action = strings.TrimSpace(action)

	conn := h.spotify.Connect()
	ctx := r.Context()

	var err error
	switch action {
	case "play", "resume":
		err = conn.Play(ctx)
	case "pause":
		err = conn.Pause(ctx)
	case "play-pause":
		err = conn.PlayPause(ctx)
	case "next":
		err = conn.Next(ctx)
	case "prev":
		err = conn.Prev(ctx)
	case "volume":
		volStr := r.URL.Query().Get("volume")
		if volStr == "" {
			volStr = r.URL.Query().Get("level")
		}
		vol, parseErr := strconv.Atoi(volStr)
		if parseErr != nil {
			Error(w, http.StatusBadRequest, "query parameter 'volume' (0-100) is required")
			return
		}
		err = conn.SetVolume(ctx, vol)
	case "seek":
		posStr := r.URL.Query().Get("position")
		pos, parseErr := strconv.ParseInt(posStr, 10, 64)
		if parseErr != nil {
			Error(w, http.StatusBadRequest, "query parameter 'position' (milliseconds) is required")
			return
		}
		err = conn.Seek(ctx, pos)
	case "load":
		uri := strings.TrimSpace(r.URL.Query().Get("uri"))
		if uri == "" {
			Error(w, http.StatusBadRequest, "query parameter 'uri' is required (e.g. spotify:track:...)")
			return
		}
		play := r.URL.Query().Get("play") != "false"
		err = conn.Load(ctx, uri, play)
	default:
		Error(w, http.StatusBadRequest, "unknown player action: "+action)
		return
	}

	if err != nil {
		if errors.Is(err, spotify.ErrConnectUnavailable) {
			Error(w, http.StatusServiceUnavailable, "Spotify Connect daemon not running")
			return
		}
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"action": action,
	})
}

// SpotifyConnectInfoHandler handles GET /api/v1/spotify/connect/info
func (h *Handler) SpotifyConnectInfoHandler(w http.ResponseWriter, r *http.Request) {
	if h.spotify == nil {
		Error(w, http.StatusServiceUnavailable, "Spotify not initialized")
		return
	}

	sup := h.spotify.Supervisor()
	conn := h.spotify.Connect()

	binPath, binType := "", ""
	installed := false
	if sup != nil {
		installed = sup.IsInstalled()
		binPath, binType = sup.BinaryInfo()
	}

	reachable := false
	baseURL := ""
	if conn != nil {
		reachable = conn.IsAvailable(r.Context())
		baseURL = conn.BaseURL()
	}

	JSON(w, http.StatusOK, map[string]any{
		"daemon_reachable":     reachable,
		"daemon_base_url":      baseURL,
		"binary_installed":     installed,
		"binary_path":          binPath,
		"binary_type":          binType,
		"install_instructions": sup.InstallInstructions(),
	})
}
