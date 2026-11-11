// Command server runs the geofencing & real-time alert API.
//
// Startup order: config → database → migrations → WebSocket hub → HTTP server.
// On Ctrl+C / SIGTERM (e.g. `docker stop`) it shuts down gracefully: it stops
// accepting new requests, lets in-flight ones finish, then closes WebSockets.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"geofence/internal/config"
	"geofence/internal/db"
	"geofence/internal/httpapi"
	"geofence/internal/realtime"
	"geofence/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogging(cfg.LogLevel)

	// ctx is cancelled when the process receives Ctrl+C or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, nil)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	st := store.New(pool)

	// The hub runs in its own goroutine for the life of the server.
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	hub := realtime.NewHub()
	go hub.Run(hubCtx)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.NewHandler(st, hub, httpapi.Options{
			AllowedOrigins: cfg.AllowedOrigins,
			APIKey:         cfg.APIKey,
			RateLimitRPS:   cfg.RateLimitRPS,
			RateLimitBurst: cfg.RateLimitBurst,
			Ping:           st.Ping,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ListenAndServe blocks, so it runs in a goroutine; errors come back on a channel.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "allowed_origins", cfg.AllowedOrigins, "api_key_enabled", cfg.APIKey != "")
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown incomplete", "err", err)
	}
	stopHub() // closes all WebSocket connections
	slog.Info("server stopped")
	return nil
}

func setupLogging(level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
