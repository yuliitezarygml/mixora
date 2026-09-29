package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var ErrPlaybackUnavailable = errors.New("SoundCloud HLS playback unavailable")

type Playback struct {
	URL    string `json:"url"`
	Format string `json:"format"`
	Source string `json:"source"`
}

// Playback resolves API redirects on the server. Authorization is only sent to
// the configured API origin; the player receives a temporary CDN URL.
func (c *Client) Playback(ctx context.Context, id string) (Playback, error) {
	data, err := c.Streams(ctx, id)
	if err != nil {
		return Playback{}, err
	}
	var streams map[string]string
	if err = json.Unmarshal(data, &streams); err != nil {
		return Playback{}, errors.New("invalid SoundCloud streams response")
	}
	target := streams["hls_aac_160_url"]
	if target == "" {
		target = streams["hls_aac_96_url"]
	}
	if target == "" {
		target = streams["hls_mp3_128_url"]
	}
	if target == "" {
		return Playback{}, ErrPlaybackUnavailable
	}
	u, err := url.Parse(target)
	base, _ := url.Parse(c.baseURL)
	if err != nil {
		return Playback{}, errors.New("invalid SoundCloud stream URL")
	}
	if isStreamCDNURL(u) {
		return Playback{URL: u.String(), Format: "hls", Source: "soundcloud"}, nil
	}
	if !isStreamAPIURL(u, base) {
		return Playback{}, errors.New("untrusted SoundCloud stream URL")
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return Playback{}, err
	}
	// A stream format may redirect to another endpoint on the same API before
	// resolving to a CDN. Bound the chain and validate every hop explicitly.
	for hop := 0; hop < 4; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return Playback{}, err
		}
		req.Header.Set("Authorization", "OAuth "+token)
		res, err := c.http.Do(req)
		if err != nil {
			return Playback{}, errors.New("SoundCloud playback resolution failed")
		}
		res.Body.Close()
		switch res.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		default:
			return Playback{}, &APIError{res.StatusCode}
		}
		location := res.Header.Get("Location")
		if location == "" {
			return Playback{}, errors.New("missing SoundCloud stream redirect")
		}
		reference, err := url.Parse(location)
		if err != nil {
			return Playback{}, errors.New("invalid SoundCloud stream redirect")
		}
		next := u.ResolveReference(reference)
		if isStreamAPIURL(next, base) {
			u = next
			continue
		}
		if isStreamCDNURL(next) {
			return Playback{URL: next.String(), Format: "hls", Source: "soundcloud"}, nil
		}
		// Origins are useful diagnostics; signed query parameters are not logged.
		return Playback{}, fmt.Errorf("untrusted SoundCloud CDN origin: scheme=%q host=%q", next.Scheme, next.Host)
	}
	return Playback{}, errors.New("too many SoundCloud stream redirects")
}

// SoundCloud's AAC HLS migration uses this exact host in addition to the
// legacy sndcdn.com CDN: https://github.com/soundcloud/api/issues/441.
// CDN URLs are returned to the player, never fetched with an OAuth header.
func isStreamCDNURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") &&
		(host == "playback.media-streaming.soundcloud.cloud" || strings.HasSuffix(host, ".sndcdn.com"))
}

func isStreamAPIURL(u, base *url.URL) bool {
	return u.Scheme == base.Scheme && u.Host == base.Host && u.User == nil &&
		strings.HasPrefix(u.Path, "/tracks/") && strings.Contains(u.Path, "/streams/")
}
