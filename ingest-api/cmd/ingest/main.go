package main

import (
	"context"
	"errors"
	"net"
	stdhttp "net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	grpcadapter "github.com/songhieu/EchoProxy/ingest-api/internal/adapter/grpc"
	httpadapter "github.com/songhieu/EchoProxy/ingest-api/internal/adapter/http"
	"github.com/songhieu/EchoProxy/ingest-api/internal/adapter/cache"
	"github.com/songhieu/EchoProxy/ingest-api/internal/adapter/kafka"
	"github.com/songhieu/EchoProxy/ingest-api/internal/adapter/postgres"
	"github.com/songhieu/EchoProxy/ingest-api/internal/infra"
	"github.com/songhieu/EchoProxy/ingest-api/internal/usecase"
	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/pkg/ratelimit"
	"github.com/songhieu/EchoProxy/pkg/redact"
)

func main() {
	cfg := infra.Load()
	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Str("service", "ingest-api").Logger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool := postgres.MustConnect(ctx, cfg.PostgresDSN)
	defer pool.Close()
	repo := postgres.NewAPIKeyRepo(pool)

	c, err := cache.New()
	if err != nil {
		log.Fatal().Err(err).Msg("cache")
	}

	prod, err := event.NewProducer(event.ProducerConfig{
		Brokers:  cfg.KafkaBrokers,
		Topic:    cfg.KafkaTopic,
		ClientID: "ingest-api",
	})
	if err != nil {
		log.Fatal().Err(err).Msg("kafka producer")
	}
	defer prod.Close()

	sink := kafka.New(prod, log)
	redactor := redact.New(redact.Rules{})
	var limiter *ratelimit.Limiter
	if cfg.RedisAddr != "" {
		l, err := ratelimit.New(cfg.RedisAddr, "rl:ingest:")
		if err != nil {
			log.Fatal().Err(err).Msg("ratelimit init")
		}
		limiter = l
		defer limiter.Close()
	} else {
		limiter = ratelimit.Disabled()
	}
	uc := usecase.NewIngest(repo, c, sink, redactor, limiter)

	httpHandler := httpadapter.New(uc).Routes()
	httpSrv := &stdhttp.Server{Addr: cfg.HTTPAddr, Handler: httpHandler, ReadHeaderTimeout: 5 * time.Second}

	grpcSrv := grpc.NewServer()
	event.RegisterEventIngestServer(grpcSrv, grpcadapter.NewServer(uc))

	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("http listening")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http")
		}
	}()
	go func() {
		ln, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			log.Fatal().Err(err).Msg("grpc listen")
		}
		log.Info().Str("addr", cfg.GRPCAddr).Msg("grpc listening")
		if err := grpcSrv.Serve(ln); err != nil {
			log.Fatal().Err(err).Msg("grpc serve")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	grpcSrv.GracefulStop()
	_ = prod.Flush(shutdownCtx)
}
