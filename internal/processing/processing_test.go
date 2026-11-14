package processing

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

type brokenQueue struct{ Queue }

type abortedQueue struct{ Queue }

func (q abortedQueue) Enqueue(ctx context.Context, tx pgx.Tx, id string) error {
	if err := q.Queue.Enqueue(ctx, tx, id); err != nil {
		return err
	}
	// A database-side transaction abort before commit must not become acceptance.
	_, _ = tx.Exec(ctx, `SELECT 1/0`)
	return nil
}

func (brokenQueue) Enqueue(context.Context, pgx.Tx, string) error {
	return errors.New("controlled queue failure")
}

func TestDurableIngestionAndWorker(t *testing.T) {
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
	if err = platform.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store, connection := platform.ID(), platform.ID()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO shops.stores(id,name) VALUES($1,'queue test')`, store)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,'fake',$1,$1,'connected')`, connection, store)
	must(err)
	defer func() {
		pool.Exec(ctx, `DELETE FROM processing.events WHERE store_id=$1`, store)
		pool.Exec(ctx, `DELETE FROM instagram.connections WHERE store_id=$1`, store)
		pool.Exec(ctx, `DELETE FROM shops.stores WHERE id=$1`, store)
	}()
	q := PostgresQueue{Pool: pool}
	s := Service{Pool: pool, Queue: q}
	e := Event{ExternalID: platform.ID(), SenderID: "customer", OccurredAt: time.Now().Add(time.Second), Scenario: "message"}
	s.Queue = brokenQueue{q}
	if _, err = s.Accept(ctx, store, e); err == nil {
		t.Fatal("enqueue failure reported success")
	}
	s.Queue = abortedQueue{q}
	if _, err = s.Accept(ctx, store, e); err == nil {
		t.Fatal("aborted database transaction reported success")
	}
	items, err := s.List(ctx, store)
	must(err)
	if len(items) != 0 {
		t.Fatal("event committed without job")
	}
	s.Queue = q
	old := e
	old.OccurredAt = time.Unix(0, 0)
	result, err := s.Accept(ctx, store, old)
	must(err)
	if result != "ignored" {
		t.Fatal(result)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Accept(ctx, store, e)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	items, err = s.List(ctx, store)
	must(err)
	if len(items) != 1 || items[0].Status != "pending" || items[0].Speaker != "customer" {
		t.Fatal(items)
	}
	j, err := q.Claim(ctx)
	must(err)
	if _, err = q.Claim(ctx); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unexpired work claimed twice", err)
	}
	// Simulate process loss after claim. A new queue instance recovers the expired lease.
	_, err = pool.Exec(ctx, `UPDATE processing.jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE event_id=$1`, j.ID)
	must(err)
	restarted := PostgresQueue{Pool: pool}
	reclaimed, err := restarted.Claim(ctx)
	must(err)
	if reclaimed.ID != j.ID || reclaimed.Token == j.Token || reclaimed.Attempts != 2 {
		t.Fatal(reclaimed)
	}
	if err = q.Ack(ctx, j); !errors.Is(err, ErrLease) {
		t.Fatal("stale worker acknowledged", err)
	}
	must(restarted.Ack(ctx, reclaimed))
	items, err = s.List(ctx, store)
	must(err)
	if items[0].Status != "succeeded" {
		t.Fatal(items)
	}
	shop := e
	shop.ExternalID = platform.ID()
	shop.SenderID = connection
	shop.Scenario = "shop"
	_, err = s.Accept(ctx, store, shop)
	must(err)
	must(ProcessOne(ctx, q))
	items, err = s.List(ctx, store)
	must(err)
	if items[0].Speaker != "shop" {
		t.Fatal(items)
	}
	failure := e
	failure.ExternalID = platform.ID()
	failure.Scenario = "failure"
	_, err = s.Accept(ctx, store, failure)
	must(err)
	for range 3 {
		must(ProcessOne(ctx, q))
		_, err = pool.Exec(ctx, `UPDATE processing.jobs SET available_at=clock_timestamp() WHERE event_id=(SELECT id FROM processing.events WHERE store_id=$1 AND external_id=$2) AND status='pending'`, store, failure.ExternalID)
		must(err)
	}
	items, err = s.List(ctx, store)
	must(err)
	if items[0].Status != "failed" || items[0].Attempts != 3 {
		t.Fatal(items)
	}
	_, err = pool.Exec(ctx, `UPDATE instagram.connections SET status='disconnected' WHERE id=$1`, connection)
	must(err)
	if _, err = s.Accept(ctx, store, Event{}); !errors.Is(err, ErrConnection) {
		t.Fatal(err)
	}
}
