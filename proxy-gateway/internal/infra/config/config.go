package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr          string
	AdminAddr           string
	KafkaBrokers        []string
	KafkaTopic          string
	PostgresDSN         string
	RedisAddr           string
	BodyCapBytes        int
	EventChanSize       int
	KafkaWorkers        int
	APIKeyRefresh       time.Duration
	UpstreamTimeout     time.Duration
	StreamIdleTimeout   time.Duration
	AllowPrivateTargets bool
}

func Load() Config {
	return Config{
		ListenAddr:      env("LISTEN_ADDR", ":8080"),
		AdminAddr:       env("ADMIN_ADDR", ":6060"),
		KafkaBrokers:    splitCSV(env("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:      env("KAFKA_TOPIC", "http_events"),
		PostgresDSN:     env("POSTGRES_DSN", "postgres://echoproxy:echoproxy@localhost:5432/echoproxy?sslmode=disable"),
		RedisAddr:       env("REDIS_ADDR", ""),
		BodyCapBytes:    envInt("BODY_CAP_BYTES", 64*1024),
		EventChanSize:   envInt("EVENT_CHAN_SIZE", 100_000),
		KafkaWorkers:    envInt("KAFKA_WORKERS", 8),
		APIKeyRefresh:   time.Duration(envInt("APIKEY_REFRESH_SECONDS", 10)) * time.Second,
		UpstreamTimeout:     time.Duration(envInt("UPSTREAM_TIMEOUT_SECONDS", 60)) * time.Second,
		StreamIdleTimeout:   time.Duration(envInt("STREAM_IDLE_TIMEOUT_SECONDS", 120)) * time.Second,
		AllowPrivateTargets: envBool("ALLOW_PRIVATE_TARGETS", false),
	}
}

func envBool(k string, def bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		return v == "1" || v == "true" || v == "yes"
	}
	return def
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
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
