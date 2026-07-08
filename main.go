package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	sm, err := NewSandboxManager()
	if err != nil {
		log.Fatal(err)
	}

	store := NewStore()
	api := NewAPI(sm, store)
	
	r := chi.NewRouter()
	r.Use(middleware.Logger)


	r.Get("/health", func(w http.ResponseWriter, r *http.Request){
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Route("/sandboxes", func(r chi.Router) {
		r.Post("/", api.CreateSandbox)
		r.Get("/", api.ListSandboxes)
		r.Get("/{id}", api.GetSandbox)
		r.Delete("/{id}", api.DeleteSandbox)
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}