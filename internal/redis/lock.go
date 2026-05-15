package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// AcquireLock attempts to acquire a distributed lock with the given key and TTL.
// Returns true if the lock was acquired, false otherwise.
func AcquireLock(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (bool, error) {
	ok, err := client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ReleaseLock releases a distributed lock by deleting the key.
func ReleaseLock(ctx context.Context, client *redis.Client, key string) error {
	return client.Del(ctx, key).Err()
}
