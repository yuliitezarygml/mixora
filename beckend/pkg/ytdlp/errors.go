package ytdlp

import "errors"

var (
	// ErrBinaryNotFound is returned when yt-dlp is not installed on the system.
	ErrBinaryNotFound = errors.New("ytdlp: binary not found in PATH (install via 'brew install yt-dlp' or 'apt install yt-dlp')")

	// ErrExtractionFailed is returned when yt-dlp fails to extract media from the URL.
	ErrExtractionFailed = errors.New("ytdlp: extraction failed")

	// ErrUnsupportedURL is returned when the given URL is invalid or empty.
	ErrUnsupportedURL = errors.New("ytdlp: unsupported or empty URL")
)
