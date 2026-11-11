// Package config reads all runtime settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string   // HTTP port (PORT). Most Docker hosts inject this.
	DatabaseURL    string   // postgres://user:pass@host:port/db?sslmode=...
	AllowedOrigins []string // CORS + WebSocket origin allow-list. "*" allows any origin.
	APIKey         string   // Optional. When set, write requests need header X-API-Key.
	RateLimitRPS   float64  // Location updates per second allowed per vehicle.
	RateLimitBurst int      // Short bursts allowed above RateLimitRPS.
	LogLevel       string   // debug | info | warn | error
}

func Load() (Config, error) {
	cfg := Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		AllowedOrigins: splitCSV(getenv("ALLOWED_ORIGINS", "http://localhost:5173")),
		APIKey:         os.Getenv("API_KEY"),
		LogLevel:       getenv("LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}

	var err error
	if cfg.RateLimitRPS, err = strconv.ParseFloat(getenv("RATE_LIMIT_RPS", "5"), 64); err != nil || cfg.RateLimitRPS <= 0 {
		return cfg, fmt.Errorf("RATE_LIMIT_RPS must be a positive number")
	}
	if cfg.RateLimitBurst, err = strconv.Atoi(getenv("RATE_LIMIT_BURST", "10")); err != nil || cfg.RateLimitBurst <= 0 {
		return cfg, fmt.Errorf("RATE_LIMIT_BURST must be a positive integer")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimRight(strings.TrimSpace(part), "/"); p != "" {
			out = append(out, p)
		}
	}
	return out
}
