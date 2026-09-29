package models

// PaginatedResponse is a generic wrapper for SoundCloud paginated collections.
type PaginatedResponse[T any] struct {
	Collection   []T    `json:"collection"`
	NextHref     string `json:"next_href,omitempty"`
	QueryURN     string `json:"query_urn,omitempty"`
	TotalResults int    `json:"total_results,omitempty"`
}

// Badges represents status badges for a creator or user.
type Badges struct {
	Pro          bool `json:"pro"`
	ProUnlimited bool `json:"pro_unlimited"`
	Verified     bool `json:"verified"`
}

// StreamURLResponse represents the response containing the playable stream URL.
type StreamURLResponse struct {
	URL string `json:"url"`
}
