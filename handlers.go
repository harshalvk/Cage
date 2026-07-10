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

type ExecRequest struct {
	Cmd []string `json:"cmd"`
}

type WriteFileRequest struct {
	Path string `json:"path"`
	Content string `json:"content"` // for now plain text; base64 later for binary
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

func (a *API) ExecCommand(w http.ResponseWriter, r *http.Request){
	id := chi.URLParam(r, "id")
	sb, ok := a.store.Get(id)
	if !ok {
		http.Error(w, "sandbox not found", http.StatusNotFound)
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
	sb, ok := a.store.Get(id)
	if !ok {
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
	sb, ok := a.store.Get(id)
	if !ok {
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