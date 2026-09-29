package models

import "time"

// ChartResponse represents the trending or top charts response.
type ChartResponse struct {
	Genre       string      `json:"genre"`
	Kind        string      `json:"kind"` // "trending" or "top"
	LastUpdated time.Time   `json:"last_updated"`
	Collection  []ChartItem `json:"collection"`
	NextHref    string      `json:"next_href,omitempty"`
}

// ChartItem wraps a track inside the chart collection.
type ChartItem struct {
	Track Track   `json:"track"`
	Score float64 `json:"score,omitempty"`
}
