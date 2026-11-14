package platform

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIdentityConstraintDamage(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated real PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "ALTER TABLE identity.users DROP CONSTRAINT users_username_key"); err != nil {
		t.Fatal(err)
	}
	if err := validate(ctx, tx); err == nil {
		t.Fatal("damaged uniqueness must fail schema validation")
	}
	// Rollback preserves the constraint and all existing accounts.
}

func TestMembershipConstraintDamage(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated real PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "ALTER TABLE shops.memberships DROP CONSTRAINT memberships_store_id_key"); err != nil {
		t.Fatal(err)
	}
	if err := validate(ctx, tx); err == nil {
		t.Fatal("damaged manager uniqueness must fail schema validation")
	}
}

func TestProcessingDedupeConstraintDamage(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated real PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "ALTER TABLE processing.events DROP CONSTRAINT events_connection_id_external_id_key"); err != nil {
		t.Fatal(err)
	}
	if err := validate(ctx, tx); err == nil {
		t.Fatal("damaged inbound dedupe must fail validation")
	}
}
