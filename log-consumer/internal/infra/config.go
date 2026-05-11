package infra

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	KafkaBrokers    []string
	KafkaTopic      string
	KafkaGroup      string
	ClickHouseDSN   string
	BatchSize       int
	BatchInterval   time.Duration
	AdminAddr       string
}

func Load() Config {
	return Config{
		KafkaBrokers:  splitCSV(env("KAFKA_BROKERS", "localhost:9092")),
		KafkaTopic:    env("KAFKA_TOPIC", "http_events"),
		KafkaGroup:    env("KAFKA_GROUP", "log-consumer"),
		ClickHouseDSN: env("CLICKHOUSE_DSN", "clickhouse://echoproxy:echoproxy@localhost:9000/echoproxy"),
		BatchSize:     envInt("BATCH_SIZE", 1000),
		BatchInterval: time.Duration(envInt("BATCH_INTERVAL_MS", 1000)) * time.Millisecond,
		AdminAddr:     env("ADMIN_ADDR", ":6061"),
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
