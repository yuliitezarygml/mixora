package api

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	youtubeVideoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	providerPathID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	vkEmbedID      = regexp.MustCompile(`^-?[0-9]+$`)
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

// The named resolver routes are intentionally narrower than /extract. They
// must not quietly turn into aliases for another provider just because yt-dlp
// happens to support both URLs.
func validateBandcampURL(rawURL string) (string, error) {
	return validateSpecificProviderURL(rawURL, isBandcampHost, "url must be an HTTPS Bandcamp track link")
}

func validateVKURL(rawURL string) (string, error) {
	return validateSpecificProviderURL(rawURL, isVKHost, "url must be an HTTPS VK or VK Video media link")
}

func validateSpecificProviderURL(rawURL string, acceptsHost func(string) bool, message string) (string, error) {
	targetURL, err := validateYTDLPURL(rawURL)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(targetURL) // validateYTDLPURL already parsed it.
	if parsed == nil || !acceptsHost(strings.ToLower(parsed.Hostname())) {
		return "", invalidMediaURL(message)
	}
	return targetURL, nil
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
		if !isYouTubeMediaPath(parsed, host) {
			return "", invalidMediaURL("url must identify one YouTube video")
		}
	} else {
		switch {
		case isYouTubeHost(host):
			if !isYouTubeMediaPath(parsed, host) {
				return "", invalidMediaURL("url must identify one YouTube video")
			}
		case isBandcampHost(host):
			if !isBandcampMediaPath(parsed) {
				return "", invalidMediaURL("url must identify one Bandcamp track")
			}
		case isVKHost(host):
			if !isVKMediaPath(parsed) {
				return "", invalidMediaURL("url must identify public VK media")
			}
		default:
			return "", invalidMediaURL("url must be an HTTPS link from YouTube, YouTube Music, Bandcamp, VK, or VK Video")
		}
	}

	return parsed.String(), nil
}

func isYouTubeHost(host string) bool {
	switch host {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be", "www.youtu.be":
		return true
	default:
		return false
	}
}

func isYouTubeMediaPath(parsed *url.URL, host string) bool {
	path := strings.Trim(parsed.EscapedPath(), "/")
	if host == "youtu.be" || host == "www.youtu.be" {
		return youtubeVideoID.MatchString(path)
	}
	if path == "watch" {
		return youtubeVideoID.MatchString(parsed.Query().Get("v"))
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return false
	}
	switch parts[0] {
	case "shorts", "live", "embed":
		return youtubeVideoID.MatchString(parts[1])
	case "clip":
		// A clip id is provider-scoped (rather than an arbitrary redirect URL)
		// and yt-dlp resolves it to its owning video.
		return providerPathID.MatchString(parts[1])
	default:
		return false
	}
}

func isBandcampHost(host string) bool {
	return host == "bandcamp.com" || strings.HasSuffix(host, ".bandcamp.com")
}

func isBandcampMediaPath(parsed *url.URL) bool {
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	return len(parts) == 2 &&
		parts[0] == "track" &&
		providerPathID.MatchString(parts[1])
}

func isVKHost(host string) bool {
	return host == "vk.com" || strings.HasSuffix(host, ".vk.com") ||
		host == "vk.ru" || strings.HasSuffix(host, ".vk.ru") ||
		host == "vkvideo.ru" || strings.HasSuffix(host, ".vkvideo.ru")
}

func isVKMediaPath(parsed *url.URL) bool {
	path := strings.Trim(parsed.EscapedPath(), "/")
	if path == "video_ext.php" {
		return vkEmbedID.MatchString(parsed.Query().Get("oid")) &&
			vkEmbedID.MatchString(parsed.Query().Get("id"))
	}
	if !providerPathID.MatchString(path) {
		return false
	}
	return strings.HasPrefix(path, "video") ||
		strings.HasPrefix(path, "clip") ||
		strings.HasPrefix(path, "wall") ||
		strings.HasPrefix(path, "audio")
}
