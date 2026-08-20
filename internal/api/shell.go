package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/harshalvk/cage/internal/backend"
	"github.com/harshalvk/cage/internal/store"
)

// resizeMessage is sent by the client as a text frame whenever the local
// terminal's size changes - distinguished from raw terminal bytes (which
// arraive as binary frames) so the two never get confused mid-stream
type resizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// OpenShellSession upgrades the connection to a WebSocket and bridges it to
// a live PTY session inside the sandbox. Binary frames in either direction
// are raw terminal bytes; text frames from the client are resize control
// messages
func (a *API) OpenShellSession(w http.ResponseWriter, r *http.Request) {
	interactive, ok := backend.AsInteractive(a.sb)
	if !ok {
		http.Error(w, "interactive shell sessions are not supported on the active isolation backend", http.StatusNotImplemented)
		return
	}

	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	sb, err := a.store.Get(r.Context(), id)
	if err != nil {
		slog.Error("shell: failed to get sandbox", "sandbox_id", id, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sb == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sb.Status != store.StatusRunning {
		http.Error(w, fmt.Sprintf("sandbox is %q, not running", sb.Status), http.StatusConflict)
		return
	}

	shell, err := interactive.OpenShell(r.Context(), sb.ID)
	if err != nil {
		slog.Error("shell: failed to open backend shell", "sandbox_id", sb.ID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := shell.Close(); cerr != nil {
			slog.Warn("shell: failed to close backend shell", "sandbox_id", sb.ID, "error", cerr)
		}
	}()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Termianl sessions are effectively binary streams - subprotocol
		// negotiation isn't needed, but accept requires expliccit config
		// to avoid overly strict default cors behaviour in dev setups
		InsecureSkipVerify: true, // TODO: tighten to real origin checks before any non-localhost deployment
	})
	if err != nil {
		slog.Error("shell: websocket upgrade failed", "sandbox_id", sb.ID, "error", err)
		return
	}
	defer func() {
		if cerr := conn.CloseNow(); cerr != nil {
			slog.Debug("shell: error closing websocket connection", "sandbox_id", sb.ID, "error", cerr)
		}
	}()

	slog.Info("shell: session opened", "sandbox_id", sb.ID)

	ctx, cancle := context.WithCancel(r.Context())
	defer cancle()

	// Pump backend shell output -> websocket, in its own goroutine
	go func() {
		buf := make([]byte, 32*1034)
		for {
			n, err := shell.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					slog.Debug("shell: write to clinet failed, ending session", "sandbox_id", sb.ID, "error", werr)
					cancle()
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					slog.Debug("shell: backend read ended", "sandbox_id", sb.ID, "error", err)
				}
				cancle()
				return
			}
		}
	}()

	// Main loop: websocket -> backend shell, on this goroutine.
	// Blocks until the client disconnects or the output pump above cancles ctx
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			slog.Debug("shell: session ended", "sandbox_id", sb.ID, "error", err)
			return
		}

		switch msgType {
		case websocket.MessageBinary:
			if _, werr := shell.Write(data); werr != nil {
				slog.Debug("shell: write to backend failed, ending session", "sandbox_id", sb.ID, "error", werr)
				return
			}
		case websocket.MessageText:
			var msg resizeMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				slog.Warn("shell: malformed control message, ignoring", "sandbox_id", sb.ID, "error", err)
				continue
			}
			if msg.Type == "resize" {
				if rerr := shell.Resize(msg.Cols, msg.Rows); rerr != nil {
					slog.Warn("shell: resize failed", "sandbox_id", sb.ID, "error", rerr)
				}
			}
		}
	}
}
