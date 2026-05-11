package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/log-consumer/internal/adapter/clickhouse"
	"github.com/songhieu/EchoProxy/log-consumer/internal/adapter/kafka"
	"github.com/songhieu/EchoProxy/log-consumer/internal/infra"
	"github.com/songhieu/EchoProxy/log-consumer/internal/usecase"
)

func main() {
	cfg := infra.Load()
	log := zerolog.New(nil).With().Timestamp().Str("service", "log-consumer").Logger()
	log = log.Output(zerologStdout())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sink, err := clickhouse.NewSink(cfg.ClickHouseDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("clickhouse")
	}
	defer sink.Close()

	for i := 0; i < 30; i++ {
		if err := sink.Ping(ctx); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	batcher := usecase.NewBatcher(sink, cfg.BatchSize, cfg.BatchInterval, log)

	consumer, err := kafka.NewConsumer(kafka.Config{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaTopic,
		Group:   cfg.KafkaGroup,
	}, batcher, log)
	if err != nil {
		log.Fatal().Err(err).Msg("kafka consumer")
	}

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		srv := &http.Server{Addr: cfg.AdminAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("admin server")
		}
	}()

	log.Info().Strs("brokers", cfg.KafkaBrokers).Str("topic", cfg.KafkaTopic).Msg("consumer running")
	consumer.Run(ctx)
	log.Info().Msg("bye")
}

func zerologStdout() zerolog.LevelWriter {
	return zerolog.MultiLevelWriter(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = time.RFC3339
	}))
}
