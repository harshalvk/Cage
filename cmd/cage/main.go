package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/harshalvk/cage/internal/api"
	"github.com/harshalvk/cage/internal/config"
	"github.com/harshalvk/cage/internal/db"
	"github.com/harshalvk/cage/internal/reaper"
	"github.com/harshalvk/cage/internal/reconcile"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	if err := db.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	store, err := store.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	sm, err := sandbox.NewSandboxManager()
	if err != nil {
		log.Fatal(err)
	}

	if err := reconcile.Reconcile(ctx, sm, store); err != nil {
		log.Printf("reconcile failed: %v", err)
	}

	reaper := reaper.NewReaper(sm, store, 5*time.Second)
	go reaper.Start(ctx)

	api := api.NewAPI(sm, store, cfg.SandboxTTL)

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
	})

	r.Route("/sandboxes", func(r chi.Router) {
		r.Post("/", api.CreateSandbox)
		r.Get("/", api.ListSandboxes)
		r.Get("/{id}", api.GetSandbox)
		r.Delete("/{id}", api.DeleteSandbox)
		r.Post("/{id}/exec", api.ExecCommand)
		r.Post("/{id}/files", api.WriteFile)
		r.Get("/{id}/files", api.ReadFile)
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
