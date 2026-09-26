package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// tasksSchema is applied on every startup; a single-table migration via
// IF NOT EXISTS is enough at this stage.
const tasksSchema = `
CREATE TABLE IF NOT EXISTS tasks (
    user_id      TEXT    NOT NULL,
    id           TEXT    NOT NULL,
    created      TEXT    NOT NULL,
    title        TEXT    NOT NULL,
    description  TEXT    NOT NULL,
    type         TEXT    NOT NULL,
    dead_line    TEXT    NOT NULL,
    dead_line_ms BIGINT  NOT NULL,
    status       TEXT    NOT NULL,
    work         TEXT    NOT NULL,
    updated_at   BIGINT  NOT NULL,
    deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (user_id, id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_user_updated ON tasks (user_id, updated_at);
`

// connectDB opens a pgx pool and applies the schema. The backend and the
// database start simultaneously under docker compose, so connection attempts
// are retried for up to ~30 seconds.
func connectDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 8

	deadline := time.Now().Add(30 * time.Second)
	for {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				if _, err = pool.Exec(ctx, tasksSchema); err != nil {
					pool.Close()
					return nil, fmt.Errorf("apply schema: %w", err)
				}
				return pool, nil
			}
			pool.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		log.Printf("waiting for database: %v", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
