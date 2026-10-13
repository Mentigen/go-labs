package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mentigen/go-labs/internal/config"
	"github.com/Mentigen/go-labs/internal/database"
	"github.com/Mentigen/go-labs/internal/httpserver"
	"github.com/Mentigen/go-labs/internal/repository"
	"github.com/Mentigen/go-labs/internal/service"
	"github.com/Mentigen/go-labs/internal/txmanager"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("load config: %v", err)
		return 1
	}

	connectContext, cancelConnect := context.WithTimeout(context.Background(), cfg.Database.ConnectTimeout)
	pool, err := database.NewPool(connectContext, cfg.Database)
	cancelConnect()

	if err != nil {
		log.Printf("connect to database: %v", err)
		return 1
	}

	log.Printf("connected to PostgreSQL")

	tripRepository := repository.NewTripRepository(pool, cfg.Database.QueryTimeout)
	transactionManager := txmanager.New(pool)
	tripService := service.NewTripService(tripRepository, transactionManager)

	handler := httpserver.NewHandler(pool.Ping, cfg.Database.QueryTimeout, tripService)

	router, err := httpserver.NewRouter(handler)
	if err != nil {
		log.Printf("build router: %v", err)
		pool.Close()
		return 1
	}

	server := httpserver.New(cfg.HTTPAddr, router, cfg.HTTPTimeouts)

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("trip-service listening on %s", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		pool.Close()

		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
			return 1
		}

	case <-signalContext.Done():
		log.Printf("shutdown signal received")

		if !shutdown(server, pool, cfg.ShutdownTimeout) {
			return 1
		}
	}

	log.Printf("trip-service stopped")

	return 0
}

func shutdown(server *http.Server, pool *pgxpool.Pool, budget time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown budget %s exceeded while waiting for requests, forcing exit: %v", budget, err)
		return false
	}

	closed := make(chan struct{})

	go func() {
		pool.Close()
		close(closed)
	}()

	select {
	case <-closed:
		return true
	case <-ctx.Done():
		log.Printf("shutdown budget %s exceeded while closing database pool, forcing exit", budget)
		return false
	}
}
