package platform

import (
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

type Config struct {
	Mode string
	DB   *pgxpool.Config
}

func LoadConfig() (Config, error) {
	mode := os.Getenv("APP_MODE")
	if mode != "demo" && mode != "live" {
		return Config{}, errors.New("APP_MODE must be demo or live")
	}
	if os.Getenv("POSTGRES_DB") != "shop_"+mode {
		return Config{}, errors.New("database must match APP_MODE")
	}
	db, err := pgxpool.ParseConfig("")
	if err != nil {
		return Config{}, errors.New("invalid database configuration")
	}
	db.ConnConfig.Host = os.Getenv("POSTGRES_HOST")
	db.ConnConfig.Port = 5432
	db.ConnConfig.Database = os.Getenv("POSTGRES_DB")
	db.ConnConfig.User = os.Getenv("POSTGRES_USER")
	db.ConnConfig.Password = os.Getenv("POSTGRES_PASSWORD")
	if file := os.Getenv("POSTGRES_PASSWORD_FILE"); file != "" {
		secret, err := os.ReadFile(file)
		if err != nil {
			return Config{}, errors.New("cannot read database secret")
		}
		db.ConnConfig.Password = strings.TrimSpace(string(secret))
	}
	if db.ConnConfig.Host == "" || db.ConnConfig.User == "" || db.ConnConfig.Password == "" {
		return Config{}, errors.New("database settings required")
	}
	db.ConnConfig.ConnectTimeout = 2 * time.Second
	db.MaxConns = 5
	return Config{Mode: mode, DB: db}, nil
}

// Migrate serializes concurrent API/worker startup and commits schema atomically.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(172901)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('infrastructure.schema_version') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, schema); err != nil {
			return err
		}
	}
	if err = validate(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func validate(ctx context.Context, db querier) error {
	var version int
	if err := db.QueryRow(ctx, "SELECT version FROM infrastructure.schema_version").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return errors.New("unsupported schema")
	}
	// Verify required columns and types, even when a version marker survives schema damage.
	var valid bool
	err := db.QueryRow(ctx, `SELECT count(*) = 5 FROM information_schema.columns WHERE table_schema = 'infrastructure' AND
 ((table_name = 'health_probe' AND column_name = 'id' AND data_type = 'uuid') OR
 (table_name = 'health_probe' AND column_name = 'checks' AND data_type = 'bigint') OR
 (table_name = 'worker_heartbeat' AND column_name = 'id' AND data_type = 'text') OR
 (table_name = 'worker_heartbeat' AND column_name = 'seen_at' AND data_type = 'timestamp with time zone') OR
 (table_name = 'schema_version' AND column_name = 'version' AND data_type = 'integer'))`).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid schema")
	}
	return nil
}

type Health struct {
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	API       string `json:"api"`
	Database  string `json:"database"`
	Worker    string `json:"worker"`
	ProbeID   string `json:"probeId,omitempty"`
	Checks    int64  `json:"checks,omitempty"`
	RequestID string `json:"requestId"`
}

func ID() string { return rand.Text() }

// Check writes only to the infrastructure probe, never to business tables.
func Check(ctx context.Context, pool *pgxpool.Pool, mode string) Health {
	h := Health{Status: "error", Mode: mode, API: "ready", Database: "error", Worker: "unknown", RequestID: ID()}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return h
	}
	defer tx.Rollback(ctx)
	if validate(ctx, tx) != nil {
		return h
	}
	const probe = "00000000-0000-4000-8000-000000000001"
	var written int64
	err = tx.QueryRow(ctx, `INSERT INTO infrastructure.health_probe(id, checks) VALUES ($1, 1)
 ON CONFLICT(id) DO UPDATE SET checks = infrastructure.health_probe.checks + 1 RETURNING checks`, probe).Scan(&written)
	if err != nil {
		return h
	}
	var read int64
	if tx.QueryRow(ctx, "SELECT checks FROM infrastructure.health_probe WHERE id=$1", probe).Scan(&read) != nil || read != written {
		return h
	}
	var worker bool
	if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM infrastructure.worker_heartbeat WHERE seen_at > clock_timestamp() - interval '10 seconds')").Scan(&worker) != nil {
		return h
	}
	if tx.Commit(ctx) != nil {
		return h
	}
	h.Database = "ready"
	h.ProbeID = probe
	h.Checks = read
	h.Worker = "preparing"
	h.Status = "preparing"
	if worker {
		h.Worker = "ready"
		h.Status = "ready"
	}
	return h
}

func Beat(ctx context.Context, pool *pgxpool.Pool) error {
	if err := validate(ctx, pool); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO infrastructure.worker_heartbeat VALUES ('worker', clock_timestamp())
 ON CONFLICT(id) DO UPDATE SET seen_at = excluded.seen_at`)
	return err
}
