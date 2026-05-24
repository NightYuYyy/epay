package redis

import (
	"context"
	"testing"
	"time"
)

func TestAcquireLockWithNilClientReturnsFalseWhenContended(t *testing.T) {
	ctx := context.Background()
	const key = "lock:nil-client-contended"
	ok, err := AcquireLock(ctx, nil, key, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock returned error for nil client: %v", err)
	}
	if !ok {
		t.Fatal("first AcquireLock returned false for nil client")
	}

	ok, err = AcquireLock(ctx, nil, key, time.Second)
	if err != nil {
		t.Fatalf("second AcquireLock returned error for nil client: %v", err)
	}
	if ok {
		t.Fatal("second AcquireLock acquired contended nil-client lock")
	}

	if err := ReleaseLock(ctx, nil, key); err != nil {
		t.Fatalf("ReleaseLock returned error for nil client: %v", err)
	}
	ok, err = AcquireLock(ctx, nil, key, time.Second)
	if err != nil {
		t.Fatalf("third AcquireLock returned error for nil client: %v", err)
	}
	if !ok {
		t.Fatal("third AcquireLock did not acquire released nil-client lock")
	}
	if err := ReleaseLock(ctx, nil, key); err != nil {
		t.Fatalf("second ReleaseLock returned error for nil client: %v", err)
	}
}

func TestReleaseLockWithNilClientIsNoop(t *testing.T) {
	if err := ReleaseLock(context.Background(), nil, "lock:test"); err != nil {
		t.Fatalf("ReleaseLock returned error for nil client: %v", err)
	}
}
