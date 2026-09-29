package soundcloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

const (
	SoundCloudWebURL = "https://soundcloud.com"
	assetsPattern    = `src="(https://a-v2\.sndcdn\.com/assets/[^"]+\.js)"`
	clientIDPattern  = `client_id:"([^"]+)"`
)

var (
	assetsRegex   = regexp.MustCompile(assetsPattern)
	clientIDRegex = regexp.MustCompile(clientIDPattern)
)

// ExtractClientID scrapes soundcloud.com scripts to extract a valid public client_id.
func ExtractClientID(ctx context.Context, hc *http.Client, userAgent string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SoundCloudWebURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: failed to fetch soundcloud homepage: %v", ErrClientIDExtraction, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read homepage: %v", ErrClientIDExtraction, err)
	}

	matches := assetsRegex.FindAllStringSubmatch(string(bodyBytes), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: no asset script tags found in HTML", ErrClientIDExtraction)
	}

	// Search scripts in reverse order because client_id is typically located in later bundles
	for i := len(matches) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		scriptURL := matches[i][1]
		sReq, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
		if err != nil {
			continue
		}
		sReq.Header.Set("User-Agent", userAgent)

		sResp, err := hc.Do(sReq)
		if err != nil {
			continue
		}

		sBody, err := io.ReadAll(sResp.Body)
		sResp.Body.Close()
		if err != nil {
			continue
		}

		idMatches := clientIDRegex.FindStringSubmatch(string(sBody))
		if len(idMatches) >= 2 && idMatches[1] != "" {
			return idMatches[1], nil
		}
	}

	return "", fmt.Errorf("%w: client_id pattern not matched in %d scripts", ErrClientIDExtraction, len(matches))
}
