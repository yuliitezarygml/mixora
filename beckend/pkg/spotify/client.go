package spotify

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client is the Spotify client providing both public metadata resolution and Spotify Connect daemon control.
type Client struct {
	clientID      string
	clientSecret  string
	accessToken   string
	userAgent     string
	httpClient    *http.Client
	connectClient *ConnectClient
	supervisor    *Supervisor
	mu            sync.RWMutex
}

// Option configures a Client.
type Option func(*Client)

// WithCredentials sets optional Spotify Developer credentials for official Web API queries.
func WithCredentials(clientID, clientSecret string) Option {
	return func(c *Client) {
		c.clientID = clientID
		c.clientSecret = clientSecret
	}
}

// WithAccessToken sets a pre-existing Spotify access token.
func WithAccessToken(token string) Option {
	return func(c *Client) {
		token = strings.TrimPrefix(token, "Bearer ")
		c.accessToken = token
	}
}

// WithConnectURL overrides the default Spotify Connect daemon base URL (default: http://127.0.0.1:24879).
func WithConnectURL(connectURL string) Option {
	return func(c *Client) {
		if connectURL != "" {
			c.connectClient = NewConnectClient(connectURL, c.httpClient)
		}
	}
}

// WithHTTPClient provides a custom http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithUserAgent customizes the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// New creates and initializes a new Spotify client.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	c := &Client{
		userAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		httpClient: &http.Client{Timeout: 15 * time.Second},
		supervisor: NewSupervisor(),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.connectClient == nil {
		c.connectClient = NewConnectClient("http://127.0.0.1:24879", c.httpClient)
	}

	return c, nil
}

// Connect returns the Spotify Connect client for player controls and status.
func (c *Client) Connect() *ConnectClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectClient
}

// Supervisor returns the daemon process supervisor.
func (c *Client) Supervisor() *Supervisor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.supervisor
}

// HTTPClient returns the underlying http.Client.
func (c *Client) HTTPClient() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.httpClient
}
