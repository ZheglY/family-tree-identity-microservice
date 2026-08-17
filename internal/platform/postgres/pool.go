package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Config struct {
	URL               string
	MaxConnections    int32
	MinConnections    int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

type Pool struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

func Open(
	ctx context.Context,
	cfg Config,
	log *zap.Logger,
) (*Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL config: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConnections
	poolConfig.MinConns = cfg.MinConnections
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	postgresPool := &Pool{
		pool: pool,
		log:  log.Named("postgres"),
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := postgresPool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	postgresPool.log.Info(
		"PostgreSQL connection established",
		zap.Int32("max_connections", cfg.MaxConnections),
		zap.Int32("min_connections", cfg.MinConnections),
	)

	return postgresPool, nil
}

func (p *Pool) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL pool: %w", err)
	}

	return nil
}

func (p *Pool) Native() *pgxpool.Pool {
	return p.pool
}

func (p *Pool) Close() {
	if p == nil || p.pool == nil {
		return
	}

	p.pool.Close()
	p.log.Info("PostgreSQL connection pool closed")
}
