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

//go:embed identity.sql
var identitySchema string

//go:embed sessions.sql
var sessionSchema string

//go:embed stores.sql
var storeSchema string

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
	var version int
	if err = tx.QueryRow(ctx, "SELECT version FROM infrastructure.schema_version").Scan(&version); err != nil {
		return err
	}
	if version == 1 {
		if err = validateVersion(ctx, tx, 1); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, identitySchema); err != nil {
			return err
		}
		version = 2
	}
	if version == 2 {
		if err = validateVersion(ctx, tx, 2); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, sessionSchema); err != nil {
			return err
		}
		version = 3
	}
	if version == 3 {
		if err = validateVersion(ctx, tx, 3); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, storeSchema); err != nil {
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
	return validateVersion(ctx, db, 4)
}

func validateVersion(ctx context.Context, db querier, expected int) error {
	var version int
	if err := db.QueryRow(ctx, "SELECT version FROM infrastructure.schema_version").Scan(&version); err != nil {
		return err
	}
	if version != expected {
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
	if expected >= 2 {
		if err := db.QueryRow(ctx, `SELECT count(*) = 4 FROM information_schema.columns WHERE table_schema='identity' AND table_name='users' AND ((column_name IN ('id','username','password_hash') AND data_type='text') OR (column_name='created_at' AND data_type='timestamp with time zone'))`).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid identity schema")
		}
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='identity.users'::regclass AND conname='users_username_key' AND contype='u' AND conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid='identity.users'::regclass AND attname='username')]::smallint[])`).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid identity uniqueness")
		}
	}
	if expected >= 3 {
		if err := db.QueryRow(ctx, `SELECT count(*)=7 FROM information_schema.columns WHERE table_schema='identity' AND ((table_name='sessions' AND ((column_name IN ('token_hash','user_id','csrf_token') AND data_type='text') OR (column_name='expires_at' AND data_type='timestamp with time zone'))) OR (table_name='login_attempts' AND ((column_name='bucket' AND data_type='text') OR (column_name='attempts' AND data_type='integer') OR (column_name='expires_at' AND data_type='timestamp with time zone'))))`).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid session schema")
		}
	}
	if expected >= 4 {
		if err := db.QueryRow(ctx, `SELECT count(*)=6 FROM information_schema.columns WHERE table_schema='shops' AND ((table_name='stores' AND ((column_name IN ('id','name') AND data_type='text') OR (column_name='created_at' AND data_type='timestamp with time zone'))) OR (table_name='memberships' AND column_name IN ('user_id','store_id','role') AND data_type='text'))`).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid store schema")
		}
		if err := db.QueryRow(ctx, `SELECT count(*)=2 FROM pg_constraint WHERE conrelid='shops.memberships'::regclass AND ((contype='p' AND conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid='shops.memberships'::regclass AND attname='user_id')]::smallint[]) OR (contype='u' AND conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid='shops.memberships'::regclass AND attname='store_id')]::smallint[]))`).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid membership uniqueness")
		}
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
