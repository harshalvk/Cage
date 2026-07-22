package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/harshalvk/cage/internal/api"
	"github.com/harshalvk/cage/internal/cache"
	"github.com/harshalvk/cage/internal/config"
	"github.com/harshalvk/cage/internal/db"
	"github.com/harshalvk/cage/internal/logging"
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

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting cage", "port", cfg.Port, "warm_pool_size", cfg.WarmPoolSize)

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
			slog.Error("failed to close container: %v", "error", err)
		}
	}()

	if err := reconcile.Reconcile(ctx, sm, store); err != nil {
		slog.Error("reconcile failed: %v", "error", err)
	}

	reaper := reaper.NewReaper(sm, store, 5*time.Second)
	go reaper.Start(ctx)

	templates, err := store.ListTemplate(ctx)
	if err != nil {
		slog.Error("failed to list templates: %v", "error", err)
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

	a := api.NewAPI(sm, store, cfg.SandboxTTL, warmPool)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(api.RequestLogger)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("failed to encode health response: %v", "error", err)
		}
	})

	r.Get("/templates", a.ListTemplates)

	r.Route("/sandboxes", func(r chi.Router) {
		r.Use(a.AuthMiddleware(store, c))
		r.Post("/", a.CreateSandbox)
		r.Get("/", a.ListSandboxes)
		r.Get("/{id}", a.GetSandbox)
		r.Delete("/{id}", a.DeleteSandbox)
		r.Post("/{id}/exec", a.ExecCommand)
		r.Post("/{id}/files", a.WriteFile)
		r.Get("/{id}/files", a.ReadFile)
		r.Post("/{id}/pause", a.PauseSandbox)
		r.Post("/{id}/resume", a.ResumeSandbox)
	})

	log.Println("listening on :8080")

	if err := http.ListenAndServe(":8080", r); err != nil {
		slog.Error("server failed: %v", "error", err)
		return
	}
}
