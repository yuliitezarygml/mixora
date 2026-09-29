// Package soundcloud accesses the official public API using application credentials.
package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrDisabled = errors.New("SoundCloud credentials are not configured")
var ErrID = errors.New("invalid SoundCloud track identifier")

type APIError struct{ Status int }

func (e *APIError) Error() string { return fmt.Sprintf("SoundCloud returned HTTP %d", e.Status) }

type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type TokenSource interface {
	AccessToken(context.Context) (string, error)
}
type Client struct {
	http    *http.Client
	baseURL string
	tokens  TokenSource
}

func New(tokens TokenSource) *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, baseURL: "https://api.soundcloud.com", tokens: tokens}
}
func (c *Client) Search(ctx context.Context, q string, limit, offset int) (json.RawMessage, error) {
	params := url.Values{"q": {q}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "linked_partitioning": {"true"}, "access": {"playable,preview,blocked"}}
	return c.get(ctx, "/tracks?"+params.Encode())
}

var trackID = regexp.MustCompile(`^(?:soundcloud:tracks:)?[0-9]{1,30}$`)

func (c *Client) Streams(ctx context.Context, id string) (json.RawMessage, error) {
	if !trackID.MatchString(id) {
		return nil, ErrID
	}
	return c.get(ctx, "/tracks/"+url.PathEscape(id)+"/streams")
}
func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	if c.tokens == nil {
		return nil, ErrDisabled
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Accept", "application/json; charset=utf-8")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SoundCloud request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, &APIError{res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("invalid SoundCloud response")
	}
	return json.RawMessage(data), nil
}
func exchange(ctx context.Context, client *http.Client, endpoint, id, secret string, old Token) (Token, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	if old.RefreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", old.RefreshToken)
		form.Set("client_id", id)
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if old.RefreshToken == "" {
		req.SetBasicAuth(id, secret)
	}
	res, err := client.Do(req)
	if err != nil {
		return Token{}, errors.New("SoundCloud token request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Token{}, &APIError{res.StatusCode}
	}
	var token Token
	if err = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&token); err != nil {
		return Token{}, errors.New("invalid SoundCloud token response")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn < 1 {
		return Token{}, errors.New("incomplete SoundCloud token response")
	}
	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return token, nil
}
