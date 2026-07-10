package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SandboxStatus string

const (
	StatusRunning SandboxStatus = "running"
	StatusStopped SandboxStatus = "stopped"
)

type Sandbox struct {
	ID          string        `json:"id"`
	ContainerID string        `json:"-"`
	Status      SandboxStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, connString string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Save(ctx context.Context, sb *Sandbox) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sandboxes (id, container_id, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET status = $3`,
		sb.ID, sb.ContainerID, sb.Status, sb.CreatedAt, sb.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save sandbox: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*Sandbox, error) {
	var sb Sandbox
	err := s.pool.QueryRow(ctx,
		`SELECT id, container_id, status, created_at, expires_at FROM sandboxes WHERE id = $1`,
		id,
	).Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("faled to get sandboxes: %w", err)
	}

	return &sb, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sandboxes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete sandbox: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]*Sandbox, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, container_id, status, created_at, expires_at FROM sandboxes ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list sandboxes: %w", err)
	}
	defer rows.Close()

	var sandboxes []*Sandbox
	for rows.Next() {
		var sb Sandbox
		if err := rows.Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan sandbox: %w", err)
		}
		sandboxes = append(sandboxes, &sb)
	}

	return sandboxes, nil
}
