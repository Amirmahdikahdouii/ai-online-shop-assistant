// Package processing owns durable inbound acceptance and leased background work.
package processing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

var ErrConnection = errors.New("connection unavailable")
var ErrLease = errors.New("lease no longer owned")

// Event is normalized by a trusted provider adapter, never from message text.
// Raw message contents and provider credentials are deliberately not retained.
type Event struct {
	ExternalID string
	SenderID   string
	OccurredAt time.Time
	Scenario   string
}

type Job struct {
	ID, Token, Scenario string
	Attempts            int
}

// Enqueue must use the caller's transaction so an event cannot commit without work.
// Claim/ack/retry are fenced by a fresh token on every lease acquisition.
type Queue interface {
	Enqueue(context.Context, pgx.Tx, string) error
	Claim(context.Context) (Job, error)
	Ack(context.Context, Job) error
	Retry(context.Context, Job) error
}

type PostgresQueue struct{ Pool *pgxpool.Pool }

func (q PostgresQueue) Enqueue(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO processing.jobs(event_id) VALUES($1)`, id)
	return err
}

func (q PostgresQueue) Claim(ctx context.Context) (Job, error) {
	j := Job{Token: platform.ID()}
	err := q.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT event_id FROM processing.jobs WHERE (status='pending' AND available_at<=clock_timestamp()) OR
 (status='processing' AND lease_until<=clock_timestamp()) ORDER BY available_at,event_id FOR UPDATE SKIP LOCKED LIMIT 1
 ), claimed AS (UPDATE processing.jobs j SET status='processing',reason='working',attempts=attempts+1,
 lease_token=$1,lease_until=clock_timestamp()+interval '30 seconds' FROM candidate c WHERE j.event_id=c.event_id
 RETURNING j.event_id,j.attempts)
 SELECT c.event_id,c.attempts,e.scenario FROM claimed c JOIN processing.events e ON e.id=c.event_id`, j.Token).Scan(&j.ID, &j.Attempts, &j.Scenario)
	return j, err
}

func (q PostgresQueue) Ack(ctx context.Context, j Job) error {
	tag, err := q.Pool.Exec(ctx, `UPDATE processing.jobs SET status='succeeded',reason='processed',lease_token=NULL,lease_until=NULL WHERE event_id=$1 AND lease_token=$2 AND status='processing' AND lease_until>clock_timestamp()`, j.ID, j.Token)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrLease
	}
	return err
}

func (q PostgresQueue) Retry(ctx context.Context, j Job) error {
	tag, err := q.Pool.Exec(ctx, `UPDATE processing.jobs SET status=CASE WHEN attempts>=3 THEN 'failed' ELSE 'pending' END,
 reason='sample_failure',available_at=clock_timestamp()+interval '2 seconds',lease_token=NULL,lease_until=NULL
 WHERE event_id=$1 AND lease_token=$2 AND status='processing' AND lease_until>clock_timestamp()`, j.ID, j.Token)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrLease
	}
	return err
}

type Service struct {
	Pool  *pgxpool.Pool
	Queue Queue
}

// Accept serializes with connection lifecycle changes, dedupes in PostgreSQL,
// and reports success only after both event and job have committed.
func (s Service) Accept(ctx context.Context, store string, e Event) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var locked, connection, page string
	var since time.Time
	if err = tx.QueryRow(ctx, `SELECT id FROM shops.stores WHERE id=$1 FOR UPDATE`, store).Scan(&locked); err != nil {
		return "", err
	}
	if err = tx.QueryRow(ctx, `SELECT id,page_id,receiving_since FROM instagram.connections WHERE store_id=$1 AND status='connected' AND provider='fake'`, store).Scan(&connection, &page, &since); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrConnection
		}
		return "", err
	}
	if e.OccurredAt.Before(since) {
		return "ignored", nil
	}
	speaker := "customer"
	if e.SenderID == page {
		speaker = "shop"
	}
	id := platform.ID()
	tag, err := tx.Exec(ctx, `INSERT INTO processing.events(id,store_id,connection_id,external_id,occurred_at,speaker,scenario) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(connection_id,external_id) DO NOTHING`, id, store, connection, e.ExternalID, e.OccurredAt, speaker, e.Scenario)
	if err != nil {
		return "", err
	}
	result := "duplicate"
	if tag.RowsAffected() == 1 {
		if err = s.Queue.Enqueue(ctx, tx, id); err != nil {
			return "", err
		}
		result = "accepted"
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return result, nil
}

// ProcessOne's only effect in this ticket is a fenced status transition. It does
// not call AI, create orders, or send messages. Reclaimed work is safe to repeat.
func ProcessOne(ctx context.Context, q Queue) error {
	j, err := q.Claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if j.Scenario == "failure" {
		return q.Retry(ctx, j)
	}
	return q.Ack(ctx, j)
}

type Status struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
	OccurredAt time.Time `json:"occurredAt"`
	Speaker    string    `json:"speaker"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	Attempts   int       `json:"attempts"`
}

func (s Service) List(ctx context.Context, store string) ([]Status, error) {
	rows, err := s.Pool.Query(ctx, `SELECT e.id,e.received_at,e.occurred_at,e.speaker,j.status,j.reason,j.attempts FROM processing.events e JOIN processing.jobs j ON j.event_id=e.id WHERE e.store_id=$1 ORDER BY e.received_at DESC,e.id DESC LIMIT 100`, store)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Status{}
	for rows.Next() {
		var item Status
		if err = rows.Scan(&item.ID, &item.ReceivedAt, &item.OccurredAt, &item.Speaker, &item.Status, &item.Reason, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
