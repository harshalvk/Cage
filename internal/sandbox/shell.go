package sandbox

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// DockerShell wraps a Docker exec session started with a real TTY
// (Tty: true), giving continuous bidirectional access to the process's
// stdin/stdout - unlike ExecCommand, which buffers a single command's
// full output before returning. this is what makes full-screen interactive
// program (vim, less, top) actually work
type DockerShell struct {
	execID   string
	hijacked types.HijackedResponse
	sm       *SandboxManager
}

// OpenShell starts an interactive shell inside the container. prefers bash
// if present (nicer editing, history), falling back to sh, which every
// base image is guranteed to have
func (sm *SandboxManager) OpenShell(ctx context.Context, containerID string) (*DockerShell, error) {
	shellCmd := []string{"/bin/sh"}
	if hasBash(ctx, sm, containerID) {
		shellCmd = []string{"/bin/bash"}
	}

	execConfig := container.ExecOptions{
		Cmd:          shellCmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}

	execCreate, err := sm.docker.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create shell exec: %w", err)
	}

	attachResp, err := sm.docker.ContainerExecAttach(ctx, execCreate.ID, container.ExecStartOptions{Tty: true})
	if err != nil {
		return nil, fmt.Errorf("failed to attach shell exec: %w", err)
	}

	return &DockerShell{execID: execCreate.ID, hijacked: attachResp, sm: sm}, nil
}

// hasBash does a quick, best-effort check for bash's presence.
// Any error (including the container not having `which` at all) just
// falls back to sh, since sh is universally available - this is a nicety,
//
//	not a requirement
func hasBash(ctx context.Context, sm *SandboxManager, containerID string) bool {
	result, err := sm.ExecCommand(ctx, containerID, []string{"which", "bash"})
	if err != nil {
		return false
	}
	return result.ExitCode == 0
}

func (s *DockerShell) Read(p []byte) (int, error) {
	return s.hijacked.Reader.Read(p)
}

func (s *DockerShell) Write(p []byte) (int, error) {
	return s.hijacked.Conn.Write(p)
}

// Resize updates the PTY's terminal size -- required for full-screen
// programs to lay out correctly, and for them to responsd correctly to the
// user's actual terminal window size changing
func (s *DockerShell) Resize(cols, rows uint16) error {
	return s.sm.docker.ContainerExecResize(context.Background(), s.execID, container.ResizeOptions{
		Height: uint(rows),
		Width:  uint(cols),
	})
}

func (s *DockerShell) Close() error {
	s.hijacked.Close()
	return nil
}
