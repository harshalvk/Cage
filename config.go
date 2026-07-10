package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	DatabaseURL    string
	ReaperInterval time.Duration
	SandboxTTL 		 time.Duration
}

func LoadConfig() (*Config, error){
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on system env vars")
	}

	cfg := &Config{
		Port: getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	reaperInterval, err := time.ParseDuration(getEnv("REAPER_INTERVAL", "30s"))
	if err != nil {
		return nil, fmt.Errorf("invalid REAPER_INTERVAL: %w", err)
	}
	cfg.ReaperInterval = reaperInterval

	sandboxTTL, err := time.ParseDuration(getEnv("SANDBOX_TTL", "1h"))
	if err != nil {
		return nil, fmt.Errorf("invalid SANDBOX_TTL: %w", err)
	}
	cfg.SandboxTTL = sandboxTTL

	return cfg, nil	
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}