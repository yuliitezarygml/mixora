package spotify

import "errors"

var (
	// ErrNotFound is returned when a Spotify track, album, artist, or playlist is not found.
	ErrNotFound = errors.New("spotify: resource not found")

	// ErrInvalidURL is returned when an invalid Spotify URL or URI is provided.
	ErrInvalidURL = errors.New("spotify: invalid URL or URI")

	// ErrUnauthorized is returned when authentication fails or token is expired.
	ErrUnauthorized = errors.New("spotify: unauthorized")

	// ErrConnectUnavailable is returned when the Spotify Connect daemon (go-librespot / librespot) is not running.
	ErrConnectUnavailable = errors.New("spotify connect: daemon not reachable (start go-librespot or librespot)")

	// ErrRateLimited is returned when Spotify rate limits the requests.
	ErrRateLimited = errors.New("spotify: rate limit exceeded")

	// ErrNoLyrics is returned when no lyrics are found for a track.
	ErrNoLyrics = errors.New("spotify: lyrics not found")
)
