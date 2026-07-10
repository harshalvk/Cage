package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/harshalvk/cage/internal/sandbox"
	"github.com/harshalvk/cage/internal/store"
)

type API struct {
	sm         *sandbox.SandboxManager
	store      *store.Store
	sandboxTTL time.Duration
}

type ExecRequest struct {
	Cmd []string `json:"cmd"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"` // for now plain text; base64 later for binary
}

func NewAPI(sm *sandbox.SandboxManager, store *store.Store, sandboxTTL time.Duration) *API {
	return &API{sm: sm, store: store, sandboxTTL: sandboxTTL}
}

func (a *API) CreateSandbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	containerID, err := a.sm.CreateSandbox(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sb := &store.Sandbox{
		ID:          uuid.NewString(),
		ContainerID: containerID,
		Status:      store.StatusRunning,
		CreatedAt:   timeNow(),
		ExpiresAt:   timeNow().Add(a.sandboxTTL),
	}

	if err := a.store.Save(r.Context(), sb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (a *API) GetSandbox(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sb); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (a *API) DeleteSandbox(w http.ResponseWriter, r *http.Request) {
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

	if err := a.store.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListSandboxes(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewEncoder(w).Encode(sandboxes); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (a *API) ExecCommand(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
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
	if path == "" {
		http.Error(w, "path query param is required", http.StatusBadRequest)
		return
	}

	content, err := a.sm.ReadFile(r.Context(), sb.ContainerID, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write(content); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}
