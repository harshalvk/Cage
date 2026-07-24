package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// releaseScript only deletes the lock if the value still matches the token
// we set — this prevents a replica from accidentally releasing a lock that
// was actually acquired by a DIFFERENT replica after our lease expired.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

// renewScript extends the TTL only if we still own the lock.
var renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    return 0
end
`)

type DistributedLock struct {
	client *redis.Client
	key    string
	token  string // unique per-acquisition, proves ownership on release/renew
	ttl    time.Duration
}

func New(client *redis.Client, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client: client,
		key:    "lock:" + key,
		token:  uuid.NewString(),
		ttl:    ttl,
	}
}

// TryAcquire attempts to become the leader. Returns false (not an error) if
// another replica already holds the lock — this is the expected common case,
// not a failure.
func (l *DistributedLock) TryAcquire(ctx context.Context) (bool, error) {
	ok, err := l.client.SetNX(ctx, l.key, l.token, l.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock acquire failed: %w", err)
	}
	return ok, nil
}

// Renew extends the lease. Call this periodically while still doing
// leader-only work, so a slow job doesn't lose leadership mid-run.
func (l *DistributedLock) Renew(ctx context.Context) (bool, error) {
	result, err := renewScript.Run(ctx, l.client, []string{l.key}, l.token, l.ttl.Microseconds()).Result()
	if err != nil {
		return false, fmt.Errorf("lock renew failed: %w", err)
	}
	renewed, _ := result.(int64)
	return renewed == 1, nil
}

// Release gives up leadership early (e.g. on graceful shutdown), only if we
// still actually hold it.
func (l *DistributedLock) Release(ctx context.Context) error {
	_, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.token).Result()
	if err != nil {
		return fmt.Errorf("lock release failed: %w", err)
	}
	return nil
}
