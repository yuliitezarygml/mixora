package gorse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/music"
)

const maxResponseBytes = 4 << 20

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Client is the small REST boundary between Mixora and Gorse. The music
// engine remains the source of playable tracks; Gorse only sees opaque user
// IDs, provider-neutral item IDs and interaction signals.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type Feedback struct {
	FeedbackType string    `json:"FeedbackType"`
	UserID       string    `json:"UserId"`
	ItemID       string    `json:"ItemId"`
	Timestamp    time.Time `json:"Timestamp"`
	Value        float64   `json:"Value"`
	Comment      string    `json:"Comment,omitempty"`
}

type item struct {
	ItemID     string         `json:"ItemId"`
	IsHidden   bool           `json:"IsHidden"`
	Categories []string       `json:"Categories"`
	Timestamp  time.Time      `json:"Timestamp"`
	Labels     map[string]any `json:"Labels"`
	Comment    string         `json:"Comment"`
}

func New(config Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("gorse base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid gorse base URL %q", config.BaseURL)
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(config.APIKey),
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) UpsertUser(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("gorse user ID is required")
	}
	body := map[string]any{"UserId": userID, "Labels": map[string]any{}, "Comment": "mixora listener"}
	return c.do(ctx, http.MethodPost, "/api/user", body, nil)
}

func (c *Client) UpsertItems(ctx context.Context, tracks []music.Track) error {
	items := make([]item, 0, len(tracks))
	seen := make(map[string]struct{}, len(tracks))
	for _, track := range tracks {
		key := track.Key()
		if strings.TrimSpace(track.Source) == "" || strings.TrimSpace(track.ID) == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		categories := []string{"source:" + track.Source}
		if genre := strings.TrimSpace(track.Genre); genre != "" {
			categories = append(categories, "genre:"+genre)
		}
		timestamp := parseTimestamp(track.CreatedAt)
		comment, _ := json.Marshal(map[string]string{
			"title": track.Title, "artist": track.Artist, "genre": track.Genre,
		})
		items = append(items, item{
			ItemID: key, IsHidden: track.Access == "blocked", Categories: categories,
			Timestamp: timestamp, Labels: map[string]any{}, Comment: string(comment),
		})
	}
	if len(items) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/api/items", items, nil)
}

// PutFeedback replaces the aggregate value in Gorse. This makes projection
// idempotent even if the API answered successfully but PostgreSQL could not
// mark the source events as exported.
func (c *Client) PutFeedback(ctx context.Context, feedback []Feedback) error {
	if len(feedback) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPut, "/api/feedback", feedback, nil)
}

// DeleteFeedback removes feedback matching one feedback type, user, and item
// from Gorse. Each path component is escaped independently so provider-neutral
// item IDs and application user IDs cannot change the requested route.
func (c *Client) DeleteFeedback(ctx context.Context, feedbackType, userID, itemID string) error {
	feedbackType = strings.TrimSpace(feedbackType)
	userID = strings.TrimSpace(userID)
	itemID = strings.TrimSpace(itemID)
	if feedbackType == "" {
		return errors.New("gorse feedback type is required")
	}
	if userID == "" {
		return errors.New("gorse user ID is required")
	}
	if itemID == "" {
		return errors.New("gorse item ID is required")
	}
	path := "/api/feedback/" + url.PathEscape(feedbackType) + "/" + url.PathEscape(userID) + "/" + url.PathEscape(itemID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) Recommend(ctx context.Context, userID string, limit int) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("gorse user ID is required")
	}
	if limit <= 0 {
		limit = 50
	}
	path := "/api/recommend/" + url.PathEscape(userID) + "?n=" + strconv.Itoa(limit)
	var raw []json.RawMessage
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		var id string
		if json.Unmarshal(value, &id) == nil && id != "" {
			result = append(result, id)
			continue
		}
		var scored struct {
			ID string `json:"Id"`
		}
		if json.Unmarshal(value, &scored) == nil && scored.ID != "" {
			result = append(result, scored.ID)
		}
	}
	return result, nil
}

func (c *Client) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode gorse request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create gorse request: %w", err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call gorse: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read gorse response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return errors.New("gorse response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(payload))
		if len(message) > 500 {
			message = message[:500]
		}
		return fmt.Errorf("gorse returned %s: %s", response.Status, message)
	}
	if output != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, output); err != nil {
			return fmt.Errorf("decode gorse response: %w", err)
		}
	}
	return nil
}

func parseTimestamp(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}
