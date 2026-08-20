package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/mdlayher/vsock"
)

const (
	agentPort = 52000 // JSON control protocol — exec/read_file/write_file
	shellPort = 52001 // raw PTY streaming — interactive shell sessions

	frameData   byte = 0x01
	frameResize byte = 0x02
)

// --- JSON control protocol (exec, read_file, write_file) ---

type Request struct {
	Type    string   `json:"type"`
	Cmd     []string `json:"cmd,omitempty"`
	Path    string   `json:"path,omitempty"`
	Content string   `json:"content,omitempty"`
}

type Response struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Content  string `json:"content,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	go startShellListener()

	l, err := vsock.Listen(agentPort, nil)
	if err != nil {
		log.Fatalf("failed to listen on vsock port %d: %v", agentPort, err)
	}
	log.Printf("guest-agent listening on vsock port %d", agentPort)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeResp(conn, Response{Error: "invalid request: " + err.Error()})
			continue
		}
		writeResp(conn, handle(req))
	}
}

func handle(req Request) Response {
	switch req.Type {
	case "exec":
		return doExec(req.Cmd)
	case "write_file":
		return doWriteFile(req.Path, req.Content)
	case "read_file":
		return doReadFile(req.Path)
	default:
		return Response{Error: "unknown request type: " + req.Type}
	}
}

func doExec(cmdArgs []string) Response {
	if len(cmdArgs) == 0 {
		return Response{Error: "cmd is required"}
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return Response{Error: err.Error()}
		}
	}
	return Response{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

func doWriteFile(path, content string) Response {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Response{Error: err.Error()}
	}
	return Response{}
}

func doReadFile(path string) Response {
	data, err := os.ReadFile(path)
	if err != nil {
		return Response{Error: err.Error()}
	}
	return Response{Content: string(data)}
}

func writeResp(w io.Writer, resp Response) {
	b, _ := json.Marshal(resp)
	if _, err := fmt.Fprintf(w, "%s\n", b); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

// --- Raw PTY shell protocol ---

// startShellListener accepts persistent shell connections on a port
// separate from the JSON control protocol — a PTY session is a raw,
// continuous byte stream, not a request/response exchange, so it needs
// its own framing rather than being shoehorned into the JSON protocol.
func startShellListener() {
	l, err := vsock.Listen(shellPort, nil)
	if err != nil {
		log.Fatalf("failed to listen on vsock shell port %d: %v", shellPort, err)
	}
	log.Printf("guest-agent shell listener on vsock port %d", shellPort)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("shell accept error: %v", err)
			continue
		}
		go handleShellConn(conn)
	}
}

func handleShellConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	shellPath := "/bin/bash"
	if _, err := os.Stat(shellPath); err != nil {
		shellPath = "/bin/sh"
	}

	cmd := exec.Command(shellPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("failed to start pty: %v", err)
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	done := make(chan struct{})

	// ptmx -> conn: PTY output streamed back to the host as data frames.
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := writeFrame(conn, frameData, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// conn -> ptmx: host keystrokes (data frames) and terminal resize
	// events (resize frames) arriving on the same stream.
	for {
		typ, payload, err := readFrame(conn)
		if err != nil {
			break
		}
		switch typ {
		case frameData:
			_, _ = ptmx.Write(payload)
		case frameResize:
			if len(payload) == 4 {
				cols := binary.BigEndian.Uint16(payload[0:2])
				rows := binary.BigEndian.Uint16(payload[2:4])
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
			}
		}
	}

	<-done
}

// writeFrame/readFrame implement a minimal length-prefixed framing:
// [1 byte type][4 byte big-endian length][payload]. Needed because raw
// vsock is just an undifferentiated byte stream — without framing, there
// would be no way to distinguish "this is PTY output" from "this is a
// resize command" on the wire.
func writeFrame(w io.Writer, typ byte, payload []byte) error {
	header := make([]byte, 5)
	header[0] = typ
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func readFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	typ := header[0]
	length := binary.BigEndian.Uint32(header[1:])
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return typ, payload, nil
}
