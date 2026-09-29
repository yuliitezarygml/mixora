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
	binPath     string
	cookiesFile string
	timeout     time.Duration
	mu          sync.RWMutex
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

// New initializes a new yt-dlp client.
func New(opts ...Option) *Client {
	c := &Client{
		timeout: 30 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.binPath == "" {
		c.binPath = findYtDlpBinary()
	}

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
	if !c.IsInstalled() {
		return nil, ErrBinaryNotFound
	}

	var cmdArgs []string
	if c.cookiesFile != "" {
		cmdArgs = append(cmdArgs, "--cookies", c.cookiesFile)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, c.binPath, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := strings.TrimSpace(stderr.String())
		if errStr == "" {
			errStr = err.Error()
		}
		return nil, fmt.Errorf("%w: %s", ErrExtractionFailed, errStr)
	}

	return stdout.Bytes(), nil
}
