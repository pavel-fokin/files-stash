package main

import (
	"log"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/pavel-fokin/files-stash/internal/server"
)

func main() {
	_ = godotenv.Load()

	cfg := server.Config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}

	srv := server.New(&cfg)

	log.Println("starting server on :8080")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}