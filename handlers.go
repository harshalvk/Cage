package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type API struct {
	sm    *SandboxManager
	store *Store
}

func NewAPI(sm *SandboxManager, store *Store) *API {
	return &API{sm: sm, store: store}
}

func (a *API) CreateSandbox(w http.ResponseWriter, r *http.Request){
	ctx := r.Context()

	containerID, err := a.sm.CreateSandbox(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb := &Sandbox{
		ID: uuid.NewString(),
		ContainerID: containerID,
		Status: StatusRunning,
		CreatedAt: timeNow(),
	}
	a.store.Save(sb)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sb)
}

func (a *API) GetSandbox(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, ok := a.store.Get(id)
	if !ok {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type","application/json")
	json.NewEncoder(w).Encode(sb)
}

func (a *API) DeleteSandbox(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, ok := a.store.Get(id)
	if !ok {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	if err := a.sm.KillSandbox(r.Context(), sb.ContainerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.store.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListSandboxes(w http.ResponseWriter, r *http.Request){
	sandboxes := a.store.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sandboxes)
}