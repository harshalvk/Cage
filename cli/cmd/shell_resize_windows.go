//go:build windows

package cmd

import (
	"context"

	"github.com/coder/websocket"
)

// watchResize is a no-op on native Windows: SIGWINCH doesn't exist there,
// and there's no equivalent console resize signal exposed to Go programs
// in a portable way. The shell still works — the remote PTY just won't
// auto-adjust if you resize your terminal window mid-session; reconnecting
// picks up the new size via sendResize's one-time check at connect time.
func watchResize(ctx context.Context, conn *websocket.Conn) {}
