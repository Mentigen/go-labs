package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mentigen/go-labs/internal/config"
	"github.com/Mentigen/go-labs/internal/container"
	"github.com/Mentigen/go-labs/internal/infra/telemetry"
)

func main() {
	os.Exit(run())
}

func run() int {
	bootstrapLogger := telemetry.NewLogger(os.Stdout, slog.LevelInfo, config.DefaultServiceName)

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load config", slog.String("error", err.Error()))
		return 1
	}

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := container.New(signalContext, cfg)
	if err != nil {
		bootstrapLogger.Error("build container", slog.String("error", err.Error()))
		return 1
	}

	logger := app.Logger

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("trip-service starting", slog.String("addr", cfg.HTTPAddr))
		serverErrors <- app.Server.ListenAndServe()
	}()

	exitCode := 0

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", slog.String("error", err.Error()))
			exitCode = 1
		}

	case <-signalContext.Done():
		logger.Info("shutdown signal received")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := app.Shutdown(shutdownContext); err != nil {
		logger.Error("shutdown budget exceeded, forcing exit",
			slog.String("budget", cfg.ShutdownTimeout.String()),
			slog.String("error", err.Error()),
		)

		return 1
	}

	logger.Info("trip-service stopped")

	return exitCode
}
