package reconcile

import (
	"context"
	"log/slog"

	"github.com/harshalvk/cage/internal/lock"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

func Reconcile(ctx context.Context, sm *sandbox.SandboxManager, st *store.Store, l *lock.DistributedLock) error {
	acquired, err := l.TryAcquire(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		slog.Info("reconcile: another replica is already reconciling, skipping")
		return nil
	}
	defer func() {
		if err := l.Release(ctx); err != nil {
			slog.Error("reconcile: failed to release lock", "error", err)
		}
	}()

	all, err := st.List(ctx)
	if err != nil {
		return err
	}

	for _, sb := range all {
		if sb.Status != store.StatusRunning {
			continue
		}

		running, err := sm.IsRunning(ctx, sb.ContainerID)
		if err != nil {
			slog.Error("reconcile: failed to check sandbox %s: %v", "sandbox_id", sb.ID, "error", err)
		}
		if !running {
			slog.Info("reconcile: sanbox %s marked running in DB but container is gone — cleaning up", "sandbox_id", sb.ID)

			if err := st.Delete(ctx, sb.ID); err != nil {
				slog.Error("reconcile: failed to delete sandbox %s: %v", "sandbox_id", sb.ID, "error", err)
			}
		}
	}
	return nil
}
