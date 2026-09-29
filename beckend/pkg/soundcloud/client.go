package soundcloud

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is the main SoundCloud v2 API client.
type Client struct {
	clientID   string
	authToken  string
	userAgent  string
	httpClient *http.Client
	mu         sync.RWMutex
}

// Option is a functional option for configuring Client.
type Option func(*Client)

// WithClientID manually sets the client_id instead of automatically scraping it.
func WithClientID(id string) Option {
	return func(c *Client) {
		c.clientID = id
	}
}

// WithAuthToken sets an optional OAuth token for accessing protected user data.
func WithAuthToken(token string) Option {
	return func(c *Client) {
		token = strings.TrimPrefix(token, "OAuth ")
		c.authToken = token
	}
}

// WithHTTPClient provides a custom http.Client (e.g. for proxies, rate-limiting, tracing).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithUserAgent customizes the User-Agent header used for web scraping and API requests.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// New initializes a SoundCloud v2 API client.
// If client_id is not passed via WithClientID, it will automatically scrape and extract
// a valid client_id from SoundCloud assets.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		userAgent:  DefaultUA,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.clientID == "" {
		id, err := ExtractClientID(ctx, c.httpClient, c.userAgent)
		if err != nil {
			return nil, err
		}
		c.clientID = id
	}

	return c, nil
}

// ClientID returns the current client_id in use.
func (c *Client) ClientID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID
}

// SetAuthToken updates the OAuth token on the client.
func (c *Client) SetAuthToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authToken = strings.TrimPrefix(token, "OAuth ")
}
