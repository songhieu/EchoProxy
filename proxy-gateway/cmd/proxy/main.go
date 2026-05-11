package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/songhieu/EchoProxy/pkg/event"
	"github.com/songhieu/EchoProxy/pkg/ratelimit"
	"github.com/songhieu/EchoProxy/pkg/redact"
	httpadapter "github.com/songhieu/EchoProxy/proxy-gateway/internal/adapter/http"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/adapter/cache"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/adapter/kafka"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/adapter/postgres"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/infra/config"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/infra/logger"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/infra/metrics"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/infra/server"
	"github.com/songhieu/EchoProxy/proxy-gateway/internal/usecase"
)

func main() {
	cfg := config.Load()
	log := logger.New("proxy-gateway")
	rec := metrics.NewRecorder()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool := postgres.MustConnect(ctx, cfg.PostgresDSN)
	defer pool.Close()

	c, err := cache.NewRistrettoCache()
	if err != nil {
		log.Fatal().Err(err).Msg("init cache")
	}

	repo := postgres.NewAPIKeyRepo(pool)
	loader := postgres.NewLoader(repo, c, cfg.APIKeyRefresh, log)
	if err := loader.Initial(ctx); err != nil {
		log.Warn().Err(err).Msg("initial apikey load failed; continuing with empty cache")
	}
	go loader.Run(ctx)

	prod, err := event.NewProducer(event.ProducerConfig{
		Brokers:  cfg.KafkaBrokers,
		Topic:    cfg.KafkaTopic,
		ClientID: "proxy-gateway",
	})
	if err != nil {
		log.Fatal().Err(err).Msg("kafka producer")
	}
	defer prod.Close()

	if err := prod.Ping(ctx); err != nil {
		log.Warn().Err(err).Msg("kafka ping failed; producer will reconnect lazily")
	}

	sink := kafka.NewSink(prod, kafka.SinkConfig{
		BufferSize: cfg.EventChanSize,
		Workers:    cfg.KafkaWorkers,
	}, log)
	sink.Run(ctx)

	transport := server.NewUpstreamTransport(cfg.UpstreamTimeout)
	usecase.AllowPrivateTargets = cfg.AllowPrivateTargets
	redactor := redact.New(redact.Rules{})
	var limiter *ratelimit.Limiter
	if cfg.RedisAddr != "" {
		l, err := ratelimit.New(cfg.RedisAddr, "rl:proxy:")
		if err != nil {
			log.Fatal().Err(err).Msg("ratelimit init")
		}
		limiter = l
		defer limiter.Close()
	} else {
		limiter = ratelimit.Disabled()
	}
	validateUC := usecase.NewValidateAPIKey(repo, c)
	proxyUC := usecase.NewProxyRequest(transport, sink, cfg.BodyCapBytes, rec, redactor, cfg.StreamIdleTimeout)
	handler := httpadapter.NewProxyHandler(validateUC, proxyUC, limiter)

	var ready uint32
	atomic.StoreUint32(&ready, 1)

	adminMux := server.NewAdminMux(
		func() bool { return atomic.LoadUint32(&ready) == 1 },
		func() server.AdminConfigView {
			return server.AdminConfigView{
				UpstreamTimeoutSeconds:   int(cfg.UpstreamTimeout / time.Second),
				StreamIdleTimeoutSeconds: int(cfg.StreamIdleTimeout / time.Second),
				BodyCapBytes:             cfg.BodyCapBytes,
				AllowPrivateTargets:      cfg.AllowPrivateTargets,
			}
		},
	)

	mainSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	adminSrv := &http.Server{
		Addr:              cfg.AdminAddr,
		Handler:           adminMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.AdminAddr).Msg("admin listening")
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("admin server error")
		}
	}()
	go func() {
		log.Info().Str("addr", cfg.ListenAddr).Msg("proxy listening")
		if err := mainSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("proxy server error")
		}
	}()

	<-ctx.Done()
	atomic.StoreUint32(&ready, 0)
	log.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = mainSrv.Shutdown(shutdownCtx)
	_ = adminSrv.Shutdown(shutdownCtx)
	_ = prod.Flush(shutdownCtx)
	log.Info().Msg("bye")
	_ = os.Stdout.Sync()
}
