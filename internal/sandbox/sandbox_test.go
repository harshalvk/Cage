package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
	args := m.Called(ctx, ref, options)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	args := m.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)
	return args.Get(0).(container.CreateResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error) {
	args := m.Called(ctx, containerID, config)
	return args.Get(0).(container.ExecCreateResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerExecAttach(ctx context.Context, execID string, config container.ExecStartOptions) (types.HijackedResponse, error) {
	args := m.Called(ctx, execID, config)
	return args.Get(0).(types.HijackedResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error) {
	args := m.Called(ctx, execID)
	return args.Get(0).(container.ExecInspect), args.Error(1)
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(container.InspectResponse), args.Error(1)
}

func (m *MockDockerClient) CopyToContainer(ctx context.Context, containerID, path string, content io.Reader, options container.CopyToContainerOptions) error {
	args := m.Called(ctx, containerID, path, content, options)
	return args.Error(0)
}

func (m *MockDockerClient) CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, container.PathStat, error) {
	args := m.Called(ctx, containerID, srcPath)
	return args.Get(0).(io.ReadCloser), args.Get(1).(container.PathStat), args.Error(2)
}

func (m *MockDockerClient) ContainerCommit(ctx context.Context, containerID string, options container.CommitOptions) (types.IDResponse, error) {
	args := m.Called(ctx, containerID, options)
	return args.Get(0).(types.IDResponse), args.Error(1)
}

func (m *MockDockerClient) ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	args := m.Called(ctx, imageID, options)
	return args.Get(0).([]image.DeleteResponse), args.Error(1)
}

func TestCreateSandbox_Success(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ImagePull", mock.Anything, "alpine:latest", mock.Anything).Return(io.NopCloser(strings.NewReader("")), nil)

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(container.CreateResponse{ID: "fake-container-id"}, nil)

	mockClient.On("ContainerStart", mock.Anything, "fake-container-id", mock.Anything).Return(nil)

	sm := NewSandboxManagerWithClient(mockClient)

	id, err := sm.CreateSandbox(context.Background(), "alpine:latest")

	require.NoError(t, err)
	assert.Equal(t, "fake-container-id", id)
	mockClient.AssertExpectations(t)
}

func TestPauseSandbox_Success(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ContainerCommit", mock.Anything, "container-1", mock.Anything).Return(types.IDResponse{ID: "image-abc"}, nil)

	mockClient.On("ContainerStop", mock.Anything, "container-1", mock.Anything).Return(nil)

	mockClient.On("ContainerRemove", mock.Anything, "container-1", mock.Anything).Return(nil)

	sm := NewSandboxManagerWithClient(mockClient)

	imageID, err := sm.PauseSandbox(context.Background(), "container-1")

	require.NoError(t, err)
	assert.Equal(t, "image-abc", imageID)
	mockClient.AssertExpectations(t)
}

func TestPauseSandbox_CommitFails(t *testing.T) {
	mockclient := new(MockDockerClient)

	mockclient.On("ContainerCommit", mock.Anything, "container-1", mock.Anything).Return(types.IDResponse{}, errors.New("commit failed"))

	sm := NewSandboxManagerWithClient(mockclient)

	_, err := sm.PauseSandbox(context.Background(), "container-1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to commit container")

	// container stop/remove should never be called if commit fails
	mockclient.AssertNotCalled(t, "ContainerStop", mock.Anything, mock.Anything, mock.Anything)
}

func TestPauseSandbox_StopFails_DoesNotRemove(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ContainerCommit", mock.Anything, "container-1", mock.Anything).Return(types.IDResponse{ID: "image-abc"}, nil)

	mockClient.On("ContainerStop", mock.Anything, "container-1", mock.Anything).Return(errors.New("stop failed"))

	sm := NewSandboxManagerWithClient(mockClient)

	_, err := sm.PauseSandbox(context.Background(), "container-1")

	assert.Error(t, err)

	// if stop fails, we should never attempt to remove the container - verifies fail-safe ordering
	mockClient.AssertNotCalled(t, "ContainerRemove", mock.Anything, mock.Anything, mock.Anything)
}

func TestResumeSandbox_Success(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(container.CreateResponse{ID: "new-container-1"}, nil)

	mockClient.On("ContainerStart", mock.Anything, "new-container-1", mock.Anything).Return(nil)

	sm := NewSandboxManagerWithClient(mockClient)

	containerID, err := sm.ResumeSandbox(context.Background(), "image-abc")

	require.NoError(t, err)
	assert.Equal(t, "new-container-1", containerID)
	mockClient.AssertExpectations(t)
}

func TestResumeSandbox_CreateFails(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(container.CreateResponse{}, errors.New("create failed"))

	sm := NewSandboxManagerWithClient(mockClient)

	_, err := sm.ResumeSandbox(context.Background(), "image-abc")

	assert.Error(t, err)
	mockClient.AssertNotCalled(t, "ContainerStart", mock.Anything, mock.Anything, mock.Anything)
}

func TestRemoveImage_Success(t *testing.T) {
	mockClient := new(MockDockerClient)

	mockClient.On("ImageRemove", mock.Anything, "image-abc", mock.Anything).Return([]image.DeleteResponse{}, nil)

	sm := NewSandboxManagerWithClient(mockClient)

	err := sm.RemoveImage(context.Background(), "image-abc")

	require.NoError(t, err)
	mockClient.AssertExpectations(t)
}
