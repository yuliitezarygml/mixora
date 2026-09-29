package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	APIBaseURL = "https://api-v2.soundcloud.com"
	DefaultUA  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:130.0) Gecko/20100101 Firefox/130.0"
)

// apiGet performs a GET request against api-v2.soundcloud.com with client_id and optional auth token.
func (c *Client) apiGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}

	c.mu.RLock()
	clientID := c.clientID
	authToken := c.authToken
	c.mu.RUnlock()

	params.Set("client_id", clientID)

	endpoint := APIBaseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "OAuth "+authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("soundcloud: HTTP %d: %s", resp.StatusCode, string(body))
	}
}

// decodeJSON decodes JSON data into target struct.
func decodeJSON[T any](data []byte) (T, error) {
	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		return target, fmt.Errorf("soundcloud: json unmarshal failed: %w", err)
	}
	return target, nil
}
