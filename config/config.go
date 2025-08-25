package config

import (
	"log"
	"os"
)

// Config хранит все переменные окружения для проекта.
type Config struct {
	DatabaseURL string
}

// Load загружает конфигурацию из .env или переменных окружения.
func Load() *Config {

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	return &Config{
		DatabaseURL: dsn,
	}
}
