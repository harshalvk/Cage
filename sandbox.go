package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type SandboxManager struct {
	docker *client.Client
}

type ExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	ExitCode int	`json:"exit_code"`
}

func NewSandboxManager() (*SandboxManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &SandboxManager{docker: cli}, nil
}

// this function pulls the base image (if not present) and starts a container
func (sm *SandboxManager) CreateSandbox(ctx context.Context) (string, error){
	imageRef := "alpine:latest" // start simple, swap later

	// pull image
	reader, err := sm.docker.ImagePull(ctx, imageRef, image.PullOptions{})

	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	defer reader.Close()
	io.Copy(io.Discard, reader) // drain pull progress output

	// crate container — sleep infinity keeps it alive so we can exec into it later
	resp, err := sm.docker.ContainerCreate(ctx, &container.Config{
		Image: imageRef,
		Cmd: []string{"sleep", "infinity"},
		Tty: false,
	}, nil, nil, nil, "")

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := sm.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

func (sm *SandboxManager)	KillSandbox(ctx context.Context, id string) error {
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
		Cmd: cmd,
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
		Stdout: stdout,
		Stderr: stderr,
		ExitCode: inspect.ExitCode,
	}, nil

}

func (sm *SandboxManager) WriteFile(ctx context.Context, containerID, destPath string, content []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: destPath,
		Mode: 0644,
		Size: int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
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

func (sm *SandboxManager) ReadFile(ctx context.Context, containerID, srcPath string)([]byte, error){
	reader, _, err := sm.docker.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file from container: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)

	_, err = tr.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read tar entry: %w", err)
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		return  nil, fmt.Errorf("failed to read file content: %w", err)
	}
	return content, nil
}

func (sm *SandboxManager) IsRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := sm.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		if client.IsErrNotFound(err){
			return false, nil //container doesn't exists at all
		}
		return  false, err
	}
	return inspect.State.Running, nil
}