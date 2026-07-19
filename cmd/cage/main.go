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
	"github.com/harshalvk/cage/internal/cache"
	"github.com/harshalvk/cage/internal/config"
	"github.com/harshalvk/cage/internal/db"
	"github.com/harshalvk/cage/internal/pool"
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

	c, err := cache.New(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("failed to close container: %v", err)
		}
	}()

	if err := reconcile.Reconcile(ctx, sm, store); err != nil {
		log.Printf("reconcile failed: %v", err)
	}

	reaper := reaper.NewReaper(sm, store, 5*time.Second)
	go reaper.Start(ctx)

	templates, err := store.ListTemplate(ctx)
	if err != nil {
		log.Printf("failed to list templates: %v", err)
		return
	}

	poolConfigs := make([]pool.TemplateConfig, 0, len(templates))
	for _, t := range templates {
		poolConfigs = append(poolConfigs, pool.TemplateConfig{
			Slug:  t.Slug,
			Image: t.Image,
			Size:  cfg.WarmPoolSize,
		})
	}

	warmPool := pool.New(sm, poolConfigs)
	warmPool.Start(ctx)
	log.Println("warm pool ready, starting server")

	api := api.NewAPI(sm, store, cfg.SandboxTTL, warmPool)

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
	})

	r.Get("/templates", api.ListTemplates)

	r.Route("/sandboxes", func(r chi.Router) {
		r.Use(api.AuthMiddleware(store, c))
		r.Post("/", api.CreateSandbox)
		r.Get("/", api.ListSandboxes)
		r.Get("/{id}", api.GetSandbox)
		r.Delete("/{id}", api.DeleteSandbox)
		r.Post("/{id}/exec", api.ExecCommand)
		r.Post("/{id}/files", api.WriteFile)
		r.Get("/{id}/files", api.ReadFile)
		r.Post("/{id}/pause", api.PauseSandbox)
		r.Post("/{id}/resume", api.ResumeSandbox)
	})

	log.Println("listening on :8080")

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Printf("server failed: %v", err)
		return
	}
}
