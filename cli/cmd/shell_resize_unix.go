//go:build !windows

package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

func watchResize(ctx context.Context, conn *websocket.Conn) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
				if err != nil {
					continue
				}
				writeResizeMessage(ctx, conn, cols, rows)
			}
		}
	}()
}
