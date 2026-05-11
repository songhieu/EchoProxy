package main

import (
	"context"
	"errors"
	stdhttp "net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	httpadapter "echoproxy/auth-api/internal/adapter/http"
	"echoproxy/auth-api/internal/adapter/postgres"
	"echoproxy/auth-api/internal/infra"
	"echoproxy/auth-api/internal/usecase"
)

func main() {
	cfg := infra.Load()
	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Str("service", "auth-api").Logger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool := postgres.MustConnect(ctx, cfg.PostgresDSN)
	defer pool.Close()

	users := postgres.NewUserRepo(pool)
	projects := postgres.NewProjectRepo(pool)
	keys := postgres.NewAPIKeyRepo(pool)

	authUC := usecase.NewAuth(users, cfg.JWTSecret, cfg.JWTTTL)
	projUC := usecase.NewProjects(projects)
	keysUC := usecase.NewAPIKeys(keys, projects)

	h := httpadapter.NewHandler(authUC, users, projUC, keysUC, log)

	srv := &stdhttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("auth-api listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Fatal().Err(err).Msg("http server")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
