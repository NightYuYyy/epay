package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// IsDuplicate checks whether a request with the given idempotency key has already been processed.
// It uses SetNX internally: returns true if the key already exists (duplicate request),
// false if the key was successfully created (first request).
func IsDuplicate(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (bool, error) {
	ok, err := client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	// SetNX returns true when the key was set (first request), false when key already exists (duplicate).
	return !ok, nil
}
