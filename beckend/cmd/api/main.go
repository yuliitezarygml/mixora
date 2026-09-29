package main

import (
	"context"
	"errors"
	"log/slog"
	"mixora/beckend/internal/config"
	"mixora/beckend/internal/database"
	"mixora/beckend/internal/httpapi"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := database.Open(startup, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = database.Migrate(startup, db); err != nil {
		return err
	}
	if err = os.MkdirAll(cfg.MediaDir, 0755); err != nil {
		return err
	}
	api, err := httpapi.New(db, cfg, log)
	if err != nil {
		return err
	}
	defer api.Close()
	srv := &http.Server{Addr: cfg.Addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	stopped := make(chan error, 1)
	go func() { log.Info("Mixora API listening", "address", cfg.Addr); stopped <- srv.ListenAndServe() }()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case err = <-stopped:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ticker.C:
			clean, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := db.Exec(clean, "DELETE FROM sessions WHERE expires_at<=now()")
			cancel()
			if err != nil {
				log.Error("session cleanup failed", "error", err)
			}
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdown); err != nil {
				_ = srv.Close()
				return err
			}
			return nil
		}
	}
}
