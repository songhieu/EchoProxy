package infra

import (
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	GRPCAddr     string
	KafkaBrokers []string
	KafkaTopic   string
	PostgresDSN  string
	RedisAddr    string
}

func Load() Config {
	return Config{
		HTTPAddr:     env("HTTP_ADDR", ":8081"),
		GRPCAddr:     env("GRPC_ADDR", ":8082"),
		KafkaBrokers: splitCSV(env("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:   env("KAFKA_TOPIC", "http_events"),
		PostgresDSN:  env("POSTGRES_DSN", "postgres://echoproxy:echoproxy@localhost:5432/echoproxy?sslmode=disable"),
		RedisAddr:    env("REDIS_ADDR", ""),
	}
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
