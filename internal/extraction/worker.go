package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

var ErrLease = errors.New("extraction lease no longer owned")

type Task struct {
	Input
	Token    string
	Attempts int
}
type Queue interface {
	Enqueue(context.Context, pgx.Tx, string) error
	Claim(context.Context) (Task, error)
	Complete(context.Context, Task, Proposal) error
	Fail(context.Context, Task, string, bool) error
}
type PostgresQueue struct {
	Pool        *pgxpool.Pool
	Lease       time.Duration
	MaxAttempts int
}

func (q PostgresQueue) Enqueue(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO catalog.extraction_jobs(selection_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	return err
}
func (q PostgresQueue) Claim(ctx context.Context) (Task, error) {
	lease := q.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	t := Task{Token: platform.ID()}
	err := q.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT j.selection_id FROM catalog.extraction_jobs j JOIN catalog.post_selections s ON s.id=j.selection_id
 WHERE (s.status='pending' AND j.available_at<=clock_timestamp()) OR (s.status='processing' AND j.lease_until<=clock_timestamp())
 ORDER BY j.available_at,j.selection_id FOR UPDATE OF j,s SKIP LOCKED LIMIT 1
 ), claimed AS (UPDATE catalog.extraction_jobs j SET attempts=attempts+1,lease_token=$1,lease_until=clock_timestamp()+$2*interval '1 second',reason='working'
 FROM candidate c WHERE j.selection_id=c.selection_id RETURNING j.selection_id,j.attempts)
 UPDATE catalog.post_selections s SET status='processing' FROM claimed c WHERE s.id=c.selection_id RETURNING s.id,s.caption,s.published_at,c.attempts`, t.Token, lease.Seconds()).Scan(&t.SourceID, &t.Caption, &t.PublishedAt, &t.Attempts)
	return t, err
}
func (q PostgresQueue) Complete(ctx context.Context, t Task, p Proposal) error {
	if err := validate(p, t.Input); err != nil {
		return err
	}
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var store, actor string
	err = tx.QueryRow(ctx, `SELECT s.store_id,s.actor_id FROM catalog.extraction_jobs j JOIN catalog.post_selections s ON s.id=j.selection_id WHERE j.selection_id=$1 AND j.lease_token=$2 AND j.lease_until>clock_timestamp() AND s.status='processing' FOR UPDATE OF j,s`, t.SourceID, t.Token).Scan(&store, &actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLease
	}
	if err != nil {
		return err
	}
	var productID *string
	status := "blocked"
	if p.Outcome == "product" {
		id := platform.ID()
		productID = &id
		status = "completed"
		value := func(f Field) string {
			if f.State == "known" {
				return f.Values[0]
			}
			return ""
		}
		name := value(p.Name)
		var code *string
		var price *int64
		// Codes remain proposals until a human saves them, avoiding collisions with
		// existing catalog identities (matching/linking is a separate ticket).
		if p.Price.State == "known" {
			n, _ := strconv.ParseInt(p.Price.Values[0], 10, 64)
			price = &n
		}
		attrs := map[string]string{}
		for k, f := range p.Attributes {
			if f.State == "known" {
				attrs[k] = value(f)
			}
		}
		variant := map[string]any{"id": platform.ID(), "name": name, "code": code, "price": price, "attributes": attrs}
		variants, _ := json.Marshal([]any{variant})
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'draft',1,$4)`, id, store, name, variants); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"id": id, "name": name, "status": "draft", "revision": 1, "variants": []any{variant}})
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.history(product_id,revision,actor_id,snapshot) VALUES($1,1,$2,$3)`, id, actor, snapshot); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(p)
	if _, err = tx.Exec(ctx, `UPDATE catalog.post_selections SET status=$2,product_id=$3,proposal=$4 WHERE id=$1`, t.SourceID, status, productID, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE catalog.extraction_jobs SET lease_token=NULL,lease_until=NULL,reason=$2 WHERE selection_id=$1`, t.SourceID, p.Outcome); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (q PostgresQueue) Fail(ctx context.Context, t Task, reason string, retry bool) error {
	max := q.MaxAttempts
	if max <= 0 {
		max = 3
	}
	status := "failed"
	if reason == "unavailable" {
		status = "blocked"
	} else if retry && t.Attempts < max {
		status = "pending"
	}
	delay := time.Duration(1<<min(t.Attempts, 6))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	tag, err := q.Pool.Exec(ctx, `WITH failed AS (UPDATE catalog.extraction_jobs SET lease_token=NULL,lease_until=NULL,reason=$3,available_at=clock_timestamp()+$4*interval '1 second' WHERE selection_id=$1 AND lease_token=$2 AND lease_until>clock_timestamp() RETURNING selection_id) UPDATE catalog.post_selections s SET status=$5 FROM failed f WHERE s.id=f.selection_id AND s.status='processing'`, t.SourceID, t.Token, reason, delay.Seconds(), status)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrLease
	}
	return err
}

type Worker struct {
	Queue     Queue
	Extractor Extractor
	Timeout   time.Duration
}

func (w Worker) ProcessOne(ctx context.Context) error {
	t, err := w.Queue.Claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	started := time.Now()
	metadata := w.Extractor.Metadata()
	outcome := "storage_error"
	defer func() {
		var cost any = "unknown"
		if metadata.Simulated {
			cost = 0.0
		}
		slog.Info("caption extraction attempt", "duration_ms", time.Since(started).Milliseconds(), "outcome", outcome, "model", metadata.Model, "policy", metadata.Policy, "simulated", metadata.Simulated, "cost_usd", cost)
	}()
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	var p Proposal
	if utf8.RuneCountInString(t.Caption) > 12000 || t.PublishedAt.IsZero() {
		err = ErrInvalid
	} else {
		p, err = w.Extractor.Extract(attempt, t.Input)
	}
	if err == nil {
		err = attempt.Err()
	}
	cancel()
	if err == nil {
		err = validate(p, t.Input)
	}
	if err == nil {
		err = w.Queue.Complete(ctx, t, p)
		if err == nil {
			outcome = p.Outcome
		}
		return err
	}
	reason, retry := "provider_error", true
	switch {
	case errors.Is(err, ErrInvalid):
		reason, retry = "invalid_output", false
	case errors.Is(err, ErrRefusal):
		reason, retry = "refused", false
	case errors.Is(err, ErrBlocked):
		reason, retry = "unavailable", false
	case errors.Is(err, context.DeadlineExceeded):
		reason = "timeout"
	}
	outcome = reason
	// Storage settlement uses the parent context, independently of model timeout.
	return w.Queue.Fail(ctx, t, reason, retry)
}
