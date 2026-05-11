package main

import (
	"context"
	"errors"
	stdhttp "net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/songhieu/EchoProxy/stats-api/internal/adapter/clickhouse"
	httpadapter "github.com/songhieu/EchoProxy/stats-api/internal/adapter/http"
	"github.com/songhieu/EchoProxy/stats-api/internal/adapter/postgres"
	"github.com/songhieu/EchoProxy/stats-api/internal/adapter/redis"
	"github.com/songhieu/EchoProxy/stats-api/internal/infra"
	"github.com/songhieu/EchoProxy/stats-api/internal/usecase"
)

func main() {
	cfg := infra.Load()
	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Str("service", "stats-api").Logger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repo, err := clickhouse.New(cfg.ClickHouseDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("clickhouse")
	}
	defer repo.Close()
	for i := 0; i < 30; i++ {
		if err := repo.Ping(ctx); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	cache := redis.New(cfg.RedisAddr, cfg.CacheTTL)

	pg := postgres.MustConnect(ctx, cfg.PostgresDSN)
	defer pg.Close()
	audit := postgres.NewAuditLogger(pg)

	q := usecase.NewQueries(repo)
	h := httpadapter.New(q, cache, audit, cfg.JWTSecret)

	srv := &stdhttp.Server{Addr: cfg.HTTPAddr, Handler: h.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("stats-api listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http")
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
