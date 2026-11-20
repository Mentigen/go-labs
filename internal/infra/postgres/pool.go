package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mentigen/go-labs/internal/config"
)

func NewPoolConfig(cfg config.DatabaseConfig, tracer *QueryTracer) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolConfig.ConnConfig.Tracer = tracer

	return poolConfig, nil
}
