package soundcloud

import "errors"

var (
	// ErrNotFound indicates that the requested resource was not found (HTTP 404).
	ErrNotFound = errors.New("soundcloud: resource not found")

	// ErrUnauthorized indicates an invalid or missing token / client_id (HTTP 401).
	ErrUnauthorized = errors.New("soundcloud: unauthorized or invalid credentials")

	// ErrNoProgressive indicates the track does not offer a direct progressive MP3 stream.
	ErrNoProgressive = errors.New("soundcloud: no progressive stream found for track")

	// ErrNoHLS indicates the track does not offer an HLS stream.
	ErrNoHLS = errors.New("soundcloud: no HLS stream found for track")

	// ErrClientIDExtraction indicates failure when trying to extract client_id from SoundCloud assets.
	ErrClientIDExtraction = errors.New("soundcloud: failed to extract client_id from website")

	// ErrUnexpectedKind indicates the resolved resource was not of the expected type.
	ErrUnexpectedKind = errors.New("soundcloud: resolved resource kind mismatch")
)
