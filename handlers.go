package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type API struct {
	sm    		 *SandboxManager
	store 		 *Store
	sandboxTTL time.Duration
}

type ExecRequest struct {
	Cmd []string `json:"cmd"`
}

type WriteFileRequest struct {
	Path string `json:"path"`
	Content string `json:"content"` // for now plain text; base64 later for binary
}

func NewAPI(sm *SandboxManager, store *Store, sandboxTTL time.Duration) *API {
	return &API{sm: sm, store: store, sandboxTTL: sandboxTTL}
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
		ExpiresAt: timeNow().Add(a.sandboxTTL),
	}
	a.store.Save(r.Context(), sb)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sb)
}

func (a *API) GetSandbox(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type","application/json")
	json.NewEncoder(w).Encode(sb)
}

func (a *API) DeleteSandbox(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	if err := a.sm.KillSandbox(r.Context(), sb.ContainerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.store.Delete(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListSandboxes(w http.ResponseWriter, r *http.Request){
	sandboxes, err := a.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sandboxes == nil {
		http.Error(w, "sandboxes not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sandboxes)
}

func (a *API) ExecCommand(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	if err := a.sm.KillSandbox(r.Context(), sb.ContainerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Cmd) == 0 {
		http.Error(w, "cmd is required", http.StatusBadRequest)
		return
	}

	result, err := a.sm.ExecCommand(r.Context(), sb.ContainerID, req.Cmd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (a *API) WriteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}


	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	if err := a.sm.WriteFile(r.Context(), sb.ContainerID, req.Path, []byte(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ReadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sb != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}


	path := r.URL.Query().Get("path")
	if path == ""{
		http.Error(w, "path query param is required", http.StatusBadRequest)
		return
	}

	content, err := a.sm.ReadFile(r.Context(), sb.ContainerID, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
}

func (s *Store) ListExpired(ctx context.Context) ([]*Sandbox, error){
  rows, err := s.pool.Query(ctx,
	  `SELECT id, container_id, status, created_at, expires_at FROM sandboxes WHERE expires_at < now() AND status = 'running'`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired sandboxes: %w", err)
	}
	defer rows.Close()

	sandboxes := []*Sandbox{}
	for rows.Next() {
		var sb Sandbox
		if err := rows.Scan(&sb.ID, &sb.ContainerID, &sb.Status, &sb.CreatedAt, &sb.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan sandbox: %w", err)
		}
		sandboxes = append(sandboxes, &sb)
	}
	return sandboxes, nil
}