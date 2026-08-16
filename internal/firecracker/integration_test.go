//go:build kvm

// This file only compiles/runs with `go test -tags kvm ./...` — it needs
// real KVM access and a built Firecracker environment (binary, kernel,
// base rootfs), and must never run as part of the normal test suite in
// standard CI, which has neither.
package firecracker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harshalvk/cage/internal/firecracker"
)

func integrationConfig(t *testing.T) firecracker.Config {
	t.Helper()

	bin := os.Getenv("FIRECRACKER_BIN")
	kernel := os.Getenv("FIRECRACKER_KERNEL")
	rootfsDir := os.Getenv("FIRECRACKER_ROOTFS_DIR")
	runDir := t.TempDir()

	if bin == "" || kernel == "" || rootfsDir == "" {
		t.Fatal("FIRECRACKER_BIN, FIRECRACKER_KERNEL, and FIRECRACKER_ROOTFS_DIR must be set for integration tests")
	}
	if _, err := os.Stat(filepath.Join(rootfsDir, "base.ext4")); err != nil {
		t.Fatalf("base.ext4 not found in FIRECRACKER_ROOTFS_DIR: %v", err)
	}

	return firecracker.Config{
		FirecrackerBin: bin,
		KernelPath:     kernel,
		RootfsBaseDir:  rootfsDir,
		RunDir:         runDir,
		VCPUCount:      1,
		MemSizeMiB:     128,
		BootTimeout:    15 * time.Second,
	}
}

func TestIntegration_FullLifecycle(t *testing.T) {
	mgr, err := firecracker.NewFirecrackerManager(integrationConfig(t))
	require.NoError(t, err)

	ctx := context.Background()
	sandboxID := "it-" + t.Name()

	t.Log("creating real sandbox against KVM...")
	require.NoError(t, mgr.CreateSandbox(ctx, sandboxID, "base"))
	defer mgr.KillSandbox(ctx, sandboxID)

	running, err := mgr.IsRunning(ctx, sandboxID)
	require.NoError(t, err)
	assert.True(t, running)

	t.Log("running a real command...")
	stdout, _, exitCode, err := mgr.ExecCommand(ctx, sandboxID, []string{"echo", "integration-test-ok"})
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "integration-test-ok")

	t.Log("writing and reading a real file...")
	require.NoError(t, mgr.WriteFile(ctx, sandboxID, "/tmp/it.txt", "hello from integration test"))
	content, err := mgr.ReadFile(ctx, sandboxID, "/tmp/it.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello from integration test", content)
}

func TestIntegration_PauseResumeSnapshot(t *testing.T) {
	mgr, err := firecracker.NewFirecrackerManager(integrationConfig(t))
	require.NoError(t, err)

	ctx := context.Background()
	sandboxID := "it-" + t.Name()

	require.NoError(t, mgr.CreateSandbox(ctx, sandboxID, "base"))

	require.NoError(t, mgr.WriteFile(ctx, sandboxID, "/tmp/before-pause.txt", "survived a real snapshot"))

	t.Log("pausing (real snapshot + memory capture)...")
	pauseRef, err := mgr.PauseSandbox(ctx, sandboxID)
	require.NoError(t, err)
	defer mgr.RemoveImage(ctx, pauseRef)

	running, err := mgr.IsRunning(ctx, sandboxID)
	require.NoError(t, err)
	assert.False(t, running, "sandbox should not be tracked as running while paused")

	t.Log("resuming from snapshot...")
	require.NoError(t, mgr.ResumeSandbox(ctx, sandboxID, pauseRef))
	defer mgr.KillSandbox(ctx, sandboxID)

	running, err = mgr.IsRunning(ctx, sandboxID)
	require.NoError(t, err)
	assert.True(t, running)

	content, err := mgr.ReadFile(ctx, sandboxID, "/tmp/before-pause.txt")
	require.NoError(t, err)
	assert.Equal(t, "survived a real snapshot", content, "file content must survive a real pause/resume cycle")
}

func TestIntegration_OpenShell(t *testing.T) {
	mgr, err := firecracker.NewFirecrackerManager(integrationConfig(t))
	require.NoError(t, err)

	ctx := context.Background()
	sandboxID := "it-" + t.Name()

	require.NoError(t, mgr.CreateSandbox(ctx, sandboxID, "base"))
	defer mgr.KillSandbox(ctx, sandboxID)

	t.Log("opening a real interactive shell over vsock...")
	shell, err := mgr.OpenShell(ctx, sandboxID)
	require.NoError(t, err)
	defer shell.Close()

	_, err = shell.Write([]byte("echo shell-test-ok\n"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	deadline := time.Now().Add(5 * time.Second)
	var output string
	for time.Now().Before(deadline) {
		n, rerr := shell.Read(buf)
		if n > 0 {
			output += string(buf[:n])
			if assert.ObjectsAreEqual(true, contains(output, "shell-test-ok")) {
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	assert.Contains(t, output, "shell-test-ok")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
