package reconcile

import (
	"context"
	"log/slog"

	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

func Reconcile(ctx context.Context, sm *sandbox.SandboxManager, st *store.Store) error {
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
