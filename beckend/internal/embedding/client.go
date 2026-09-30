package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const maxResponseBytes = 32 << 20

type ClientConfig struct {
	BaseURL    string
	Model      string
	Dimensions int
	Timeout    time.Duration
}

type Client struct {
	baseURL    string
	model      string
	dimensions int
	http       *http.Client
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Truncate   bool     `json:"truncate"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func NewClient(config ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("embedding base URL is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("embedding model is required")
	}
	if config.Dimensions != 768 {
		return nil, fmt.Errorf("embedding dimensions must be 768, got %d", config.Dimensions)
	}
	if config.Timeout <= 0 {
		return nil, errors.New("embedding timeout must be positive")
	}
	return &Client{
		baseURL:    baseURL,
		model:      config.Model,
		dimensions: config.Dimensions,
		http:       &http.Client{Timeout: config.Timeout},
	}, nil
}

func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{
		Model: c.model, Input: inputs, Truncate: true, Dimensions: c.dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request embeddings: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("embedding service returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var decoded embedResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}
	if len(decoded.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, want %d", len(decoded.Embeddings), len(inputs))
	}
	for index, vector := range decoded.Embeddings {
		if len(vector) != c.dimensions {
			return nil, fmt.Errorf("embedding %d has %d dimensions, want %d", index, len(vector), c.dimensions)
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, fmt.Errorf("embedding %d contains a non-finite value", index)
			}
		}
	}
	return decoded.Embeddings, nil
}
