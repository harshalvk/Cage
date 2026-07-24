package lock_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harshalvk/cage/internal/lock"
)

func TestLock_SecondAcquireFailsWhileHeld(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	lockA := lock.New(client, "test-job", 10*time.Second)
	lockB := lock.New(client, "test-job", 10*time.Second)

	acquiredA, err := lockA.TryAcquire(context.Background())
	require.NoError(t, err)
	assert.True(t, acquiredA)

	acquiredB, err := lockB.TryAcquire(context.Background())
	require.NoError(t, err)
	assert.False(t, acquiredB, "second replica should not acquire an already-held lock")
}

func TestLock_AcquiredAfterExpiry(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	lockA := lock.New(client, "test-job-2", 1*time.Second)
	acquiredA, _ := lockA.TryAcquire(context.Background())
	assert.True(t, acquiredA)

	mr.FastForward(2 * time.Second) // simulate the lease expiring

	lockB := lock.New(client, "test-job-2", 1*time.Second)
	acquiredB, err := lockB.TryAcquire(context.Background())
	require.NoError(t, err)
	assert.True(t, acquiredB, "a new replica should acquire once the old lease expires")
}

func TestLock_ReleaseOnlyAffectsOwnToken(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	lockA := lock.New(client, "test-job-3", 10*time.Second)
	lockA.TryAcquire(context.Background())

	// A DIFFERENT lock instance (different token) tries to release the same key —
	// this simulates a stale replica trying to release a lock it no longer owns.
	lockB := lock.New(client, "test-job-3", 10*time.Second)
	err = lockB.Release(context.Background()) // should be a no-op, not an error, and not actually release A's lock
	require.NoError(t, err)

	// lockA should still hold it — a third party shouldn't be able to acquire yet
	lockC := lock.New(client, "test-job-3", 10*time.Second)
	acquiredC, _ := lockC.TryAcquire(context.Background())
	assert.False(t, acquiredC, "lockA's lock should be untouched by lockB's release attempt")
}
