package soundcloud

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
)

// Resource paths come from the public SoundCloud OpenAPI specification.
// Callers cannot supply an origin, arbitrary path or authenticated endpoint.
func (c *Client) Resource(ctx context.Context, kind, id, section string) (json.RawMessage, error) {
	if kind != "tracks" && kind != "users" && kind != "playlists" {
		return nil, ErrID
	}
	pattern := regexp.MustCompile(`^(?:soundcloud:` + kind + `:)?[0-9]{1,30}$`)
	if !pattern.MatchString(id) {
		return nil, ErrID
	}
	path := "/" + kind + "/" + url.PathEscape(id)
	if section != "" {
		if kind != "users" || (section != "tracks" && section != "playlists") {
			return nil, ErrID
		}
		path += "/" + section + "?linked_partitioning=true&limit=100"
	}
	return c.get(ctx, path)
}
func (c *Client) SearchCatalog(ctx context.Context, kind, q string, limit, offset int) (json.RawMessage, error) {
	if kind == "tracks" {
		return c.Search(ctx, q, limit, offset)
	}
	if kind != "users" && kind != "playlists" {
		return nil, ErrID
	}
	params := url.Values{"q": {q}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "linked_partitioning": {"true"}}
	return c.get(ctx, "/"+kind+"?"+params.Encode())
}
