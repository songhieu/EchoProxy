package infra

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr      string
	ClickHouseDSN string
	PostgresDSN   string
	RedisAddr     string
	JWTSecret     string
	CacheTTL      time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:      env("HTTP_ADDR", ":8084"),
		ClickHouseDSN: env("CLICKHOUSE_DSN", "clickhouse://echoproxy:echoproxy@localhost:9000/echoproxy"),
		PostgresDSN:   env("POSTGRES_DSN", "postgres://echoproxy:echoproxy@localhost:5432/echoproxy?sslmode=disable"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		JWTSecret:     env("JWT_SECRET", "dev_secret_change_me_in_prod_minimum_32_chars"),
		CacheTTL:      time.Duration(envInt("CACHE_TTL_SECONDS", 30)) * time.Second,
	}
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
