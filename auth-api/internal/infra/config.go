package infra

import (
	"os"
	"time"
)

type Config struct {
	HTTPAddr    string
	PostgresDSN string
	JWTSecret   string
	JWTTTL      time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":8083"),
		PostgresDSN: env("POSTGRES_DSN", "postgres://echoproxy:echoproxy@localhost:5432/echoproxy?sslmode=disable"),
		JWTSecret:   env("JWT_SECRET", "dev_secret_change_me_in_prod_minimum_32_chars"),
		JWTTTL:      24 * time.Hour,
	}
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}
