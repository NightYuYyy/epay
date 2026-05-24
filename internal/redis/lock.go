package redis

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var localLocks sync.Map

// AcquireLock attempts to acquire a distributed lock with the given key and TTL.
// Returns true if the lock was acquired, false otherwise.
func AcquireLock(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (bool, error) {
	if client == nil {
		lock, _ := localLocks.LoadOrStore(key, &sync.Mutex{})
		if !lock.(*sync.Mutex).TryLock() {
			return false, nil
		}
		return true, nil
	}
	ok, err := client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ReleaseLock releases a distributed lock by deleting the key.
func ReleaseLock(ctx context.Context, client *redis.Client, key string) error {
	if client == nil {
		if lock, ok := localLocks.Load(key); ok {
			lock.(*sync.Mutex).Unlock()
		}
		return nil
	}
	return client.Del(ctx, key).Err()
}
