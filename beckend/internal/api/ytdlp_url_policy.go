package api

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// mediaURLPolicyError marks a user-supplied link that must not be handed to
// yt-dlp. Keeping this separate from extractor errors lets every endpoint
// consistently return a client error without exposing an arbitrary egress
// primitive.
type mediaURLPolicyError struct {
	message string
}

func (e *mediaURLPolicyError) Error() string {
	return e.message
}

func invalidMediaURL(message string) error {
	return &mediaURLPolicyError{message: message}
}

func isMediaURLPolicyError(err error) bool {
	var policyErr *mediaURLPolicyError
	return errors.As(err, &policyErr)
}

// validateYTDLPURL accepts only public HTTPS URLs from the providers that the
// product supports. yt-dlp can resolve a much broader set of URLs, so this
// boundary must remain in front of every user-controlled extraction request.
func validateYTDLPURL(rawURL string) (string, error) {
	return validateProviderURL(rawURL, false)
}

// validateYouTubeURL is intentionally narrower than the universal extractor:
// the YouTube stream route should never turn into a generic remote fetch.
func validateYouTubeURL(rawURL string) (string, error) {
	return validateProviderURL(rawURL, true)
}

func validateProviderURL(rawURL string, youtubeOnly bool) (string, error) {
	targetURL := strings.TrimSpace(rawURL)
	if targetURL == "" {
		return "", invalidMediaURL("query parameter 'url' is required")
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return "", invalidMediaURL("url must be an absolute HTTPS media link")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", invalidMediaURL("url must use HTTPS")
	}
	if parsed.User != nil {
		return "", invalidMediaURL("url must not include user credentials")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", invalidMediaURL("url must use the default HTTPS port")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" || strings.HasSuffix(host, ".") {
		return "", invalidMediaURL("url host is invalid")
	}
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", invalidMediaURL("url must not target an IP address or localhost")
	}

	if youtubeOnly {
		if !isYouTubeHost(host) {
			return "", invalidMediaURL("url must be an HTTPS YouTube or YouTube Music link")
		}
	} else if !isSupportedMediaHost(host) {
		return "", invalidMediaURL("url must be an HTTPS link from YouTube, YouTube Music, Bandcamp, VK, or VK Video")
	}

	return parsed.String(), nil
}

func isSupportedMediaHost(host string) bool {
	return isYouTubeHost(host) || isBandcampHost(host) || isVKHost(host)
}

func isYouTubeHost(host string) bool {
	switch host {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be", "www.youtu.be":
		return true
	default:
		return false
	}
}

func isBandcampHost(host string) bool {
	return host == "bandcamp.com" || strings.HasSuffix(host, ".bandcamp.com")
}

func isVKHost(host string) bool {
	return host == "vk.com" || strings.HasSuffix(host, ".vk.com") || host == "vkvideo.ru" || strings.HasSuffix(host, ".vkvideo.ru")
}
