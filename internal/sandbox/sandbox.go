package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type SandboxManager struct {
	docker DockerClient
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func NewSandboxManager() (*SandboxManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &SandboxManager{docker: cli}, nil
}

func NewSandboxManagerWithClient(cli DockerClient) *SandboxManager {
	return &SandboxManager{docker: cli}
}

// this function pulls the base image (if not present) and starts a container
func (sm *SandboxManager) CreateSandbox(ctx context.Context, imageRef string) (string, error) {
	// pull image
	reader, err := sm.docker.ImagePull(ctx, imageRef, image.PullOptions{})

	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	defer func() {
		if err := reader.Close(); err != nil {
			slog.Error("failed to close reader: %v", "error", err)
		}
	}()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		slog.Error("failed to drain image pull output: %v", "error", err)
	} // drain pull progress output

	// crate container — sleep infinity keeps it alive so we can exec into it later
	resp, err := sm.docker.ContainerCreate(ctx, &container.Config{
		Image: imageRef,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   false,
	}, nil, nil, nil, "")

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := sm.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

func (sm *SandboxManager) KillSandbox(ctx context.Context, id string) error {
	timeout := 5

	if err := sm.docker.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := sm.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	return nil
}

func (sm *SandboxManager) ExecCommand(ctx context.Context, containerID string, cmd []string) (*ExecResult, error) {
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execCreate, err := sm.docker.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := sm.docker.ContainerExecAttach(ctx, execCreate.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach exec: %w", err)
	}
	defer attachResp.Close()

	stdout, stderr, err := demuxOutput(attachResp.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read exec output: %w", err)
	}

	inspect, err := sm.docker.ContainerExecInspect(ctx, execCreate.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return &ExecResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: inspect.ExitCode,
	}, nil

}

func (sm *SandboxManager) WriteFile(ctx context.Context, containerID, destPath string, content []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name:    destPath,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to `write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("failed to write tar content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	err := sm.docker.CopyToContainer(ctx, containerID, "/", &buf, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("failed to copy file to container: %w", err)
	}

	return nil
}

func (sm *SandboxManager) ReadFile(ctx context.Context, containerID, srcPath string) ([]byte, error) {
	reader, _, err := sm.docker.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file from container: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Error("failed to close container copy reader: %v", "error", err)
		}
	}()

	tr := tar.NewReader(reader)

	_, err = tr.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read tar entry: %w", err)
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}
	return content, nil
}

func (sm *SandboxManager) IsRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := sm.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil // container doesn't exist at all
		}
		return false, err
	}
	return inspect.State.Running, nil
}

// pause_sandbox snapshots the container's filesystem into a new docker image,
// and then stops and removes the container - freeing its memory entirely
// returns the id of the commited image, which resume_sandobx needs lateer
func (sm *SandboxManager) PauseSandbox(ctx context.Context, containerID string) (imageID string, err error) {
	commitResp, err := sm.docker.ContainerCommit(ctx, containerID, container.CommitOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to commit container: %w", err)
	}

	timeout := 5
	if err := sm.docker.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return "", fmt.Errorf("failed to stop container: %w", err)
	}
	if err := sm.docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return "", fmt.Errorf("failed to remove container: %w", err)
	}

	return commitResp.ID, nil
}

// resume_sandbox creates and start a fresh container from a previously
// commited image, restoring the sandbox's filesystem state
func (sm *SandboxManager) ResumeSandbox(ctx context.Context, imageID string) (contaierID string, err error) {
	resp, err := sm.docker.ContainerCreate(ctx, &container.Config{
		Image: imageID,
		Cmd:   []string{"sleep", "infinity"},
	}, nil, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("failed to create container from paused image: %w", err)
	}

	if err := sm.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start resumed container: %w", err)
	}

	return resp.ID, nil
}

// remove_image cleans up a commited pause-image once it's no longer need
// i.e after a successful resume
func (sm *SandboxManager) RemoveImage(ctx context.Context, imageID string) error {
	if _, err := sm.docker.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove image: %w", err)
	}
	return nil
}
