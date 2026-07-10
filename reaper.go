package main

import (
	"context"
	"log"
	"time"
)

type Reaper struct {
	sm       *SandboxManager
	store    *Store
	interval time.Duration
}

func NewReaper(sm *SandboxManager, store *Store, interval time.Duration) *Reaper {
	return &Reaper{sm:sm, store: store, interval: interval}
}

func (r *Reaper) Start(ctx context.Context){
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

func (r *Reaper) reap(ctx context.Context){
	expired, err := r.store.ListExpired(ctx)
	if err != nil {
		log.Printf("reaper: failed to list expired sandboxes: %v", err)
		return
	}

	for _, sb := range expired {
		log.Printf("reaper: killing expired sandbox %s", sb.ID)
		if err := r.sm.KillSandbox(ctx, sb.ContainerID); err != nil {
			log.Printf("reaper: failed to kill container for sandbox %s: %v", sb.ID, err)
			continue
		}

		if err := r.store.Delete(ctx, sb.ID); err != nil {
			log.Printf("reaper: failed to delete sandbox %s from store: %v", sb.ID, err)
		}
	}
}