package database

import (
	"context"
	"fmt"
	"log"

	"epay/ent"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

// NewClient opens a PostgreSQL connection and returns an Ent client.
// The dsn should be a PostgreSQL connection string,
// e.g. "host=localhost port=5432 user=epay dbname=epay password=epay sslmode=disable"
func NewClient(dsn string) (*ent.Client, error) {
	drv, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	client := ent.NewClient(ent.Driver(drv))

	// Run auto-migration
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to run auto-migration: %w", err)
	}

	log.Println("[database] connected and migrated successfully")
	return client, nil
}

func init() {
	// Ensure postgres driver is registered
	_ = dialect.Debug
}
