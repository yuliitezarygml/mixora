package ytdlp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client provides an interface for extracting media and streaming URLs using yt-dlp.
type Client struct {
	binPath       string
	cookiesFile   string
	timeout       time.Duration
	maxConcurrent int
	semaphore     chan struct{}
	mu            sync.RWMutex
}

// Option configures a Client.
type Option func(*Client)

// WithBinaryPath explicitly sets the path to the yt-dlp binary.
func WithBinaryPath(path string) Option {
	return func(c *Client) {
		c.binPath = path
	}
}

// WithCookiesFile provides an optional cookies file (useful for VK or private accounts).
func WithCookiesFile(path string) Option {
	return func(c *Client) {
		c.cookiesFile = path
	}
}

// WithTimeout sets the default timeout for extraction operations.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
}

// WithMaxConcurrent bounds extractor subprocesses in one API process. This
// is a resource guard, not a replacement for HTTP-level authentication.
func WithMaxConcurrent(value int) Option {
	return func(c *Client) {
		c.maxConcurrent = value
	}
}

// New initializes a new yt-dlp client.
func New(opts ...Option) *Client {
	c := &Client{
		timeout:       30 * time.Second,
		maxConcurrent: 2,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.binPath == "" {
		c.binPath = findYtDlpBinary()
	}
	if c.maxConcurrent < 1 {
		c.maxConcurrent = 1
	}
	c.semaphore = make(chan struct{}, c.maxConcurrent)

	return c
}

// IsInstalled checks if yt-dlp was located on the system.
func (c *Client) IsInstalled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.binPath != ""
}

// BinaryPath returns the resolved path to the yt-dlp binary.
func (c *Client) BinaryPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.binPath
}

// Version returns the installed yt-dlp version string.
func (c *Client) Version(ctx context.Context) (string, error) {
	if !c.IsInstalled() {
		return "", ErrBinaryNotFound
	}

	cmd := exec.CommandContext(ctx, c.binPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting yt-dlp version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func findYtDlpBinary() string {
	candidates := []string{
		"yt-dlp",
		"/opt/homebrew/bin/yt-dlp",
		"/usr/local/bin/yt-dlp",
		"/usr/bin/yt-dlp",
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		candidates = append(candidates, home+"/.local/bin/yt-dlp")
	}

	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}

	return ""
}

func (c *Client) runCommand(ctx context.Context, args ...string) ([]byte, error) {
	c.mu.RLock()
	binPath := c.binPath
	cookiesFile := c.cookiesFile
	timeout := c.timeout
	semaphore := c.semaphore
	c.mu.RUnlock()
	if binPath == "" {
		return nil, ErrBinaryNotFound
	}
	// Every extractor invocation is an external subprocess and may wait on a
	// remote provider. Honor the client timeout even when the HTTP server keeps
	// streaming/WebSocket writes open indefinitely. An earlier caller deadline
	// remains authoritative because context.WithTimeout uses the sooner one.
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrExtractionFailed, ctx.Err())
	}

	var cmdArgs []string
	if cookiesFile != "" {
		cmdArgs = append(cmdArgs, "--cookies", cookiesFile)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, binPath, cmdArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// A provider can put temporary signed URLs, cookie advice, or other
		// diagnostics on stderr. Preserve the typed cause for callers without
		// turning that untrusted output into an API response.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrExtractionFailed, ctxErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrExtractionFailed, err)
	}

	return stdout.Bytes(), nil
}
