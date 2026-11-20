package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type transactionKey struct{}

type Manager struct {
	pool    *pgxpool.Pool
	tracer  trace.Tracer
	timeout time.Duration
}

func NewManager(pool *pgxpool.Pool, tracer trace.Tracer, timeout time.Duration) *Manager {
	return &Manager{pool: pool, tracer: tracer, timeout: timeout}
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if transactionFromContext(ctx) != nil {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	ctx, span := m.tracer.Start(ctx, "transaction")
	defer span.End()

	err := m.run(ctx, fn)
	if err != nil {
		span.SetStatus(codes.Error, "transaction failed")
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.type", transactionErrorType(err)))
	}

	return err
}

func transactionErrorType(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "_OTHER"
}

func (m *Manager) run(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	transactionContext := context.WithValue(ctx, transactionKey{}, tx)

	defer func() {
		if panicValue := recover(); panicValue != nil {
			_ = tx.Rollback(ctx)
			panic(panicValue)
		}
	}()

	if err := fn(transactionContext); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func ExecutorFromContext(ctx context.Context, fallback Executor) Executor {
	if tx := transactionFromContext(ctx); tx != nil {
		return tx
	}

	return fallback
}

func transactionFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(transactionKey{}).(pgx.Tx)
	return tx
}
