package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/coder/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var shellCmd = &cobra.Command{
	Use:   "shell [sandbox-id]",
	Short: "Open an interactive shell inside a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	sandboxID := args[0]

	wsURL := strings.Replace(serverURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = fmt.Sprintf("%s/sandboxes/%s/shell", wsURL, sandboxID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+apiKey)

	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotImplemented {
			return fmt.Errorf("interactive shells are not supported on this sandbox's isolation backend")
		}
		return fmt.Errorf("failed to connect: %w", err)
	}
	if conn == nil {
		return fmt.Errorf("websocket dial returned no connection and no error — this should not happen")
	}
	defer conn.CloseNow()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set raw terminal mode: %w", err)
	}
	defer func() {
		_ = term.Restore(fd, oldState)
	}()

	sendResize(ctx, conn)
	watchResize(ctx, conn)

	// stdin -> websocket, in its own goroutine
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("shell session ended: %w", err)
		}
		if msgType == websocket.MessageBinary {
			os.Stdout.Write(data)
		}
	}
}

// sendResize sends the terminal's current size once, immediately after
// connecting, so the remote PTY starts at the correct dimensions instead
// of whatever default the backend assumed.
func sendResize(ctx context.Context, conn *websocket.Conn) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}
	writeResizeMessage(ctx, conn, cols, rows)
}

func writeResizeMessage(ctx context.Context, conn *websocket.Conn, cols, rows int) {
	msg := map[string]any{"type": "resize", "cols": cols, "rows": rows}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = conn.Write(ctx, websocket.MessageText, b)
}
