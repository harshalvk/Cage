package reaper

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/harshalvk/cage/internal/metrics"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

type Reaper struct {
	sm       *sandbox.SandboxManager
	store    *store.Store
	interval time.Duration
}

func NewReaper(sm *sandbox.SandboxManager, store *store.Store, interval time.Duration) *Reaper {
	return &Reaper{sm: sm, store: store, interval: interval}
}

func (r *Reaper) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("reaper stopped")
			return
		case <-ticker.C:
			r.reap(ctx)
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
