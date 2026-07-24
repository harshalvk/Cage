package reaper

import (
	"context"
	"log/slog"
	"time"

	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/metrics"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

type Reaper struct {
	sm       *sandbox.SandboxManager
	store    *store.Store
	interval time.Duration
	lock     *lock.DistributedLock
}

func NewReaper(sm *sandbox.SandboxManager, store *store.Store, interval time.Duration, l *lock.DistributedLock) *Reaper {
	return &Reaper{sm: sm, store: store, interval: interval, lock: l}
}

func (r *Reaper) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reaper stopped")
			return
		case <-ticker.C:
			acquired, err := r.lock.TryAcquire(ctx)
			if err != nil {
				slog.Error("reaper: lock acquire error, skipping this tick", "error", err)
				continue
			}
			if !acquired {
				// another replica is the leader this tick - nothing to do here
				continue
			}

			r.reap(ctx)

			if err := r.lock.Release(ctx); err != nil {
				slog.Error("reaper: failed to release lock", "error", err)
			}
		}
	}
}

func (r *Reaper) reap(ctx context.Context) {
	expired, err := r.store.ListExpired(ctx)
	metrics.SandboxesReaped.Inc()
	if err != nil {
		slog.Error("reaper: failed to list expired sandboxes: %v", "error", err)
		return
	}

	for _, sb := range expired {
		slog.Info("reaper: killing expired sandbox %s", "sandbox_id", sb.ID)
		if err := r.sm.KillSandbox(ctx, sb.ContainerID); err != nil {
			slog.Error("reaper: failed to kill container for sandbox %s: %v", "sandbox_id", sb.ID, "error", err)
			continue
		}

		if err := r.store.Delete(ctx, sb.ID); err != nil {
			slog.Error("reaper: failed to delete sandbox %s from store: %v", "sandbox_id", sb.ID, "error", err)
		}
	}
}
