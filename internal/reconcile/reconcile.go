package reconcile

import (
	"context"
	"log"

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
			log.Printf("reconcile: failed to check sandbox %s: %v", sb.ID, err)
		}
		if !running {
			log.Printf("reconcile: sanbox %s marked running in DB but container is gone — cleaning up", sb.ID)

			if err := st.Delete(ctx, sb.ID); err != nil {
				log.Printf("reconcile: failed to delete sandbox %s: %v", sb.ID, err)
			}
		}
	}
	return nil
}
