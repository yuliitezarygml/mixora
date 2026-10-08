package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
	"github.com/iulian/soundcloud-go/pkg/soundcloud"
	"github.com/iulian/soundcloud-go/pkg/spotify"
	"github.com/iulian/soundcloud-go/pkg/ytdlp"
)

// Handler holds the soundcloud, spotify and ytdlp clients and handlers.
type Handler struct {
	client             *soundcloud.Client
	spotify            *spotify.Client
	ytdlp              *ytdlp.Client
	tracks             TrackObserver
	mediaHTTP          *http.Client
	extractAudioSource func(context.Context, string) (*ytdlp.AudioSource, error)
	mediaStreams       chan struct{}
}

type TrackObserver interface {
	Save(context.Context, []music.Track) error
}

// NewHandler creates a new unified API Handler instance.
func NewHandler(client *soundcloud.Client, spClient *spotify.Client, ytClient *ytdlp.Client) *Handler {
	handler := &Handler{
		client:       client,
		spotify:      spClient,
		ytdlp:        ytClient,
		mediaHTTP:    newMediaHTTPClient(),
		mediaStreams: make(chan struct{}, 8),
	}
	if ytClient != nil {
		handler.extractAudioSource = ytClient.ExtractAudioSource
	}
	return handler
}

type mediaResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type mediaDialFunc func(context.Context, string, string) (net.Conn, error)

const (
	mediaDNSLookupTimeout = 5 * time.Second
	mediaDialTimeout      = 5 * time.Second
	mediaIdleIOTimeout    = 30 * time.Second
	mediaTotalTimeout     = 2 * time.Hour
)

func newMediaHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return newMediaHTTPClientWith(net.DefaultResolver, dialer.DialContext)
}

func newMediaHTTPClientWith(resolver mediaResolver, dial mediaDialFunc) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialPublicMedia(ctx, network, address, resolver, dial)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   4,
		MaxConnsPerHost:       8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		DisableCompression:    true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   mediaTotalTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many media redirects")
			}
			_, err := validateExtractedAudioURL(request.URL.String())
			return err
		},
	}
}

type mediaIdleConn struct {
	net.Conn
	idle time.Duration
}

func (c *mediaIdleConn) Read(buffer []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	return c.Conn.Read(buffer)
}

func (c *mediaIdleConn) Write(buffer []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.idle))
	return c.Conn.Write(buffer)
}

func dialPublicMedia(ctx context.Context, network, address string, resolver mediaResolver, dial mediaDialFunc) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	lookupCtx, cancelLookup := context.WithTimeout(ctx, mediaDNSLookupTimeout)
	addresses, err := resolver.LookupIPAddr(lookupCtx, host)
	cancelLookup()
	if err != nil {
		return nil, err
	}
	var lastErr error
	allowed := 0
	for _, candidate := range addresses {
		if !publicMediaIP(candidate.IP) {
			continue
		}
		if network == "tcp4" && candidate.IP.To4() == nil {
			continue
		}
		if network == "tcp6" && candidate.IP.To4() != nil {
			continue
		}
		allowed++
		dialCtx, cancelDial := context.WithTimeout(ctx, mediaDialTimeout)
		connection, dialErr := dial(dialCtx, network, net.JoinHostPort(candidate.IP.String(), port))
		cancelDial()
		if dialErr == nil {
			return &mediaIdleConn{Conn: connection, idle: mediaIdleIOTimeout}, nil
		}
		lastErr = dialErr
	}
	if allowed == 0 {
		return nil, fmt.Errorf("media host has no public address")
	}
	return nil, fmt.Errorf("media host connection failed: %w", lastErr)
}

var blockedMediaNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func publicMediaIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return false
	}
	for _, network := range blockedMediaNetworks {
		if network.Contains(address) {
			return false
		}
	}
	return true
}

func validateExtractedAudioURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("invalid extracted media URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil {
		return "", fmt.Errorf("unsafe extracted media URL")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("unsafe extracted media port")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || strings.HasSuffix(host, ".") || net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", fmt.Errorf("unsafe extracted media host")
	}
	return parsed.String(), nil
}

// SetTrackObserver lets the app layer index provider-verified results without
// changing the music engine response contract.
func (h *Handler) SetTrackObserver(observer TrackObserver) {
	h.tracks = observer
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
	h.observeSoundCloudResolve(r.Context(), raw)

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
	h.observeSoundCloudTrack(r.Context(), *track)

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
	h.observeSoundCloudTrack(r.Context(), *track)

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
	h.observeSoundCloudTrack(r.Context(), *track)

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
	h.observeSoundCloudTracks(r.Context(), res.Collection)

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
	h.observeSoundCloudTrack(r.Context(), *track)

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
	h.observeSoundCloudChart(r.Context(), charts)

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
		tracks := make([]music.Track, 0, len(res.Collection))
		for _, raw := range res.Collection {
			tracks = append(tracks, music.FromSoundCloud(raw))
		}
		h.observeTracks(r.Context(), tracks)
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
		h.observeSoundCloudSearchItems(r.Context(), res.Collection)
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
	h.observeSoundCloudTracks(r.Context(), tracks.Collection)

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
	h.observeSoundCloudPlaylist(r.Context(), *pl)

	JSON(w, http.StatusOK, pl)
}
