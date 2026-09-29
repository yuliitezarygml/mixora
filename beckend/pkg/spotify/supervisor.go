package spotify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// DaemonConfig contains configuration options for running the Spotify Connect daemon.
type DaemonConfig struct {
	DeviceName string
	Bitrate    int    // 96, 160, 320
	Port       int    // default 24879
	CacheDir   string // directory for credentials and audio cache
	Username   string // optional
	Password   string // optional
}

// Supervisor manages a local background instance of go-librespot or librespot.
type Supervisor struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	binPath string
	binType string // "go-librespot" or "librespot"
}

// NewSupervisor creates a new supervisor.
func NewSupervisor() *Supervisor {
	s := &Supervisor{}
	s.binPath, s.binType = s.findBinary()
	return s
}

// IsInstalled returns true if a supported Spotify Connect daemon binary is found on the system.
func (s *Supervisor) IsInstalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binPath != ""
}

// BinaryInfo returns the discovered binary path and type.
func (s *Supervisor) BinaryInfo() (path string, binType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binPath, s.binType
}

// IsRunning checks whether the supervised daemon is currently active.
func (s *Supervisor) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil
}

// Start launches the Spotify Connect daemon in the background.
func (s *Supervisor) Start(ctx context.Context, cfg DaemonConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.binPath == "" {
		s.binPath, s.binType = s.findBinary()
		if s.binPath == "" {
			return fmt.Errorf("no Spotify Connect daemon found. Install go-librespot (`brew install go-librespot`) or librespot (`cargo install librespot`)")
		}
	}

	if s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil {
		return fmt.Errorf("daemon is already running (PID: %d)", s.cmd.Process.Pid)
	}

	if cfg.DeviceName == "" {
		cfg.DeviceName = "Go-Music-Player"
	}
	if cfg.Port <= 0 {
		cfg.Port = 24879
	}
	if cfg.CacheDir == "" {
		home, _ := os.UserHomeDir()
		cfg.CacheDir = filepath.Join(home, ".cache", "spotify-connect")
	}
	_ = os.MkdirAll(cfg.CacheDir, 0755)

	var args []string
	if s.binType == "go-librespot" {
		args = []string{
			"--name", cfg.DeviceName,
			"--api_port", fmt.Sprintf("%d", cfg.Port),
			"--cache_dir", cfg.CacheDir,
		}
		if cfg.Bitrate > 0 {
			args = append(args, "--bitrate", fmt.Sprintf("%d", cfg.Bitrate))
		}
		if cfg.Username != "" && cfg.Password != "" {
			args = append(args, "--username", cfg.Username, "--password", cfg.Password)
		}
	} else {
		// librespot (Rust)
		args = []string{
			"--name", cfg.DeviceName,
			"--cache", cfg.CacheDir,
		}
		if cfg.Bitrate > 0 {
			args = append(args, "--bitrate", fmt.Sprintf("%d", cfg.Bitrate))
		}
		if cfg.Username != "" && cfg.Password != "" {
			args = append(args, "-u", cfg.Username, "-p", cfg.Password)
		}
	}

	cmd := exec.CommandContext(ctx, s.binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s (%s): %w", s.binType, s.binPath, err)
	}

	s.cmd = cmd

	// Wait briefly to check if it exits immediately on error
	time.Sleep(300 * time.Millisecond)
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return fmt.Errorf("daemon exited immediately with code %d", cmd.ProcessState.ExitCode())
	}

	return nil
}

// Stop gracefully stops the running daemon.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	_ = s.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-time.After(3 * time.Second):
		_ = s.cmd.Process.Kill()
	case <-done:
	}

	s.cmd = nil
	return nil
}

// InstallInstructions returns clear installation steps for the current operating system.
func (s *Supervisor) InstallInstructions() string {
	switch runtime.GOOS {
	case "darwin":
		return "On macOS, install via Homebrew:\n  brew install go-librespot\nOr via Cargo (Rust librespot):\n  cargo install librespot --features rodio-backend"
	case "linux":
		return "On Linux, install via Homebrew or APT:\n  sudo apt-get install libogg-dev libvorbis-dev libasound2-dev\n  cargo install librespot --features alsa-backend\nOr download prebuilt binary from https://github.com/devgianlu/go-librespot/releases"
	default:
		return "Download prebuilt binaries from:\n  https://github.com/devgianlu/go-librespot/releases\nor https://github.com/librespot-org/librespot/releases"
	}
}

func (s *Supervisor) findBinary() (string, string) {
	// 1. Check for go-librespot in standard locations
	candidates := []string{
		"go-librespot",
		"/opt/homebrew/bin/go-librespot",
		"/usr/local/bin/go-librespot",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, "go-librespot"
		}
	}

	// 2. Check for Rust librespot in standard locations
	rustCandidates := []string{
		"librespot",
		"/opt/homebrew/bin/librespot",
		"/usr/local/bin/librespot",
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		rustCandidates = append(rustCandidates, filepath.Join(home, ".cargo", "bin", "librespot"))
	}
	for _, c := range rustCandidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, "librespot"
		}
	}

	return "", ""
}
