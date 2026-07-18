package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func New(redisURL string) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancle := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancle()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Cache{client: client}, nil
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // cache miss, not an errror - caller check for empty string
	}
	return val, err
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}
