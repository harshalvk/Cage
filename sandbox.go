package main

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type SandboxManager struct {
	docker *client.Client
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