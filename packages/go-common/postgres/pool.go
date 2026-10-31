package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Pool struct {
	pool *pgxpool.Pool
}

type connectFunc func(ctx context.Context, config Config) (*Pool, error)

func Open(ctx context.Context, config Config) (*Pool, error) {
	return openWithRetry(ctx, config, openOnce)
}

func openWithRetry(ctx context.Context, config Config, connect connectFunc) (*Pool, error) {
	if !config.Configured() {
		return nil, ErrNotConfigured
	}

	attempts := config.RetryAttempts
	if attempts < 1 {
		attempts = 1
	}

	delay := config.RetryMin
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		pool, err := connect(ctx, config)
		if err == nil {
			return pool, nil
		}
		if errors.Is(err, ErrNotConfigured) || errors.Is(err, ErrInvalidConfig) {
			return nil, err
		}
		lastErr = err

		if attempt == attempts {
			break
		}
		if err := sleep(ctx, delay); err != nil {
			return nil, err
		}
		delay = nextDelay(delay, config.RetryMax)
	}

	return nil, fmt.Errorf("connect to database after %d attempts: %w", attempts, lastErr)
}

func openOnce(ctx context.Context, config Config) (*Pool, error) {
	parsed, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, ErrInvalidConfig
	}

	parsed.MaxConns = config.MaxConns
	parsed.MinConns = config.MinConns
	parsed.MaxConnLifetime = config.MaxConnLifetime
	parsed.MaxConnIdleTime = config.MaxConnIdleTime
	parsed.HealthCheckPeriod = config.HealthCheckPeriod
	parsed.ConnConfig.ConnectTimeout = config.ConnectTimeout

	connectCtx := ctx
	if config.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, config.ConnectTimeout)
		defer cancel()
	}

	pool, err := pgxpool.NewWithConfig(connectCtx, parsed)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Pool{pool: pool}, nil
}

func (p *Pool) Ping(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return ErrNotConfigured
	}
	return p.pool.Ping(ctx)
}

func (p *Pool) Close() {
	if p == nil || p.pool == nil {
		return
	}
	p.pool.Close()
}

// Exec exposes parameterized SQL for service-owned repositories.
func (p *Pool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if p == nil || p.pool == nil {
		return pgconn.CommandTag{}, ErrNotConfigured
	}
	return p.pool.Exec(ctx, sql, args...)
}

// QueryRow exposes a single-row query for service-owned repositories.
func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.QueryRow(ctx, sql, args...)
}

// Query exposes a row iterator for service-owned repositories.
func (p *Pool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if p == nil || p.pool == nil {
		return nil, ErrNotConfigured
	}
	return p.pool.Query(ctx, sql, args...)
}

// Begin starts a transaction for service-owned repositories.
func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) {
	if p == nil || p.pool == nil {
		return nil, ErrNotConfigured
	}
	return p.pool.Begin(ctx)
}

func nextDelay(current, maximum time.Duration) time.Duration {
	next := current * 2
	if next <= 0 {
		next = DefaultRetryMin
	}
	if maximum > 0 && next > maximum {
		return maximum
	}
	return next
}

func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
