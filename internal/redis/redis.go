// Package redis provides Redis client initialization, health check,
// distributed locking, and idempotency utilities for the epay platform.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a new Redis client and verifies connectivity with a PING.
// Returns an error if the connection cannot be established.
func NewClient(addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}

// HealthCheck verifies Redis connectivity by sending a PING.
// Returns nil if the connection is healthy.
func HealthCheck(ctx context.Context, client *redis.Client) error {
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
}
