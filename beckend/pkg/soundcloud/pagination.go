package soundcloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/iulian/soundcloud-go/pkg/soundcloud/models"
)

// FetchNext fetches the subsequent page of results from NextHref.
// Returns nil, nil if nextHref is empty.
func FetchNext[T any](ctx context.Context, c *Client, nextHref string) (*models.PaginatedResponse[T], error) {
	if nextHref == "" {
		return nil, nil
	}

	parsed, err := url.Parse(nextHref)
	if err != nil {
		return nil, err
	}

	c.mu.RLock()
	clientID := c.clientID
	authToken := c.authToken
	c.mu.RUnlock()

	q := parsed.Query()
	q.Set("client_id", clientID)
	parsed.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("soundcloud: pagination HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return decodeJSON[*models.PaginatedResponse[T]](body)
}
