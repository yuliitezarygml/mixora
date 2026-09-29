package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mixora/beckend/internal/catalog"
	"mixora/beckend/internal/config"
	"mixora/beckend/internal/database"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	source := flag.String("file", "", "audio file path")
	title := flag.String("title", "", "track title")
	artist := flag.String("artist", "", "artist name")
	album := flag.String("album", "", "album name")
	explicit := flag.Bool("explicit", false, "original explicit version")
	flag.Parse()
	*title = strings.TrimSpace(*title)
	*artist = strings.TrimSpace(*artist)
	if *source == "" || *title == "" || *artist == "" || utf8.RuneCountInString(*title) > 200 || utf8.RuneCountInString(*artist) > 200 {
		return fmt.Errorf("provide -file, -title and -artist (max 200 characters)")
	}
	ext := strings.ToLower(filepath.Ext(*source))
	switch ext {
	case ".mp3", ".wav", ".ogg", ".flac", ".m4a":
	default:
		return fmt.Errorf("supported formats: mp3, wav, ogg, flac, m4a")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		return err
	}
	in, err := os.Open(*source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 500<<20 {
		return fmt.Errorf("audio must be a regular non-empty file up to 500 MiB")
	}
	if err = os.MkdirAll(cfg.MediaDir, 0755); err != nil {
		return err
	}
	bytes := make([]byte, 16)
	if _, err = rand.Read(bytes); err != nil {
		return err
	}
	key := hex.EncodeToString(bytes) + ext
	path := filepath.Join(cfg.MediaDir, key)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		out.Close()
		if !committed {
			os.Remove(path)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	track, err := catalog.New(db).Create(ctx, catalog.Track{Title: *title, Artist: *artist, Album: *album, Explicit: *explicit, MediaKey: key})
	if err != nil {
		return err
	}
	committed = true
	return json.NewEncoder(os.Stdout).Encode(track)
}
