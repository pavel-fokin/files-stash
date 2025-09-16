package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
	"github.com/pavel-fokin/files-stash/internal/server"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, using environment variables")
	}

	// Parse configuration from environment variables
	cfg := server.Config{}
	if err := env.Parse(&cfg); err != nil {
		slog.Error("Failed to parse configuration", "error", err)
		os.Exit(1)
	}

	// Create a new server
	srv := server.New(&cfg)

	// Start the server
	slog.Info("Starting server on :8080")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
