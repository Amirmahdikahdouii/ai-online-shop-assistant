package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/extraction"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
)

type brokenExtractionQueue struct{ extraction.Queue }

func (q brokenExtractionQueue) Enqueue(context.Context, pgx.Tx, string) error {
	return errors.New("queue unavailable")
}

type extractorFunc func(context.Context, extraction.Input) (extraction.Proposal, error)

func (f extractorFunc) Extract(ctx context.Context, in extraction.Input) (extraction.Proposal, error) {
	return f(ctx, in)
}

func TestCaptionExtraction(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated PostgreSQL")
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
	user, store, connection, token, csrf := platform.ID(), platform.ID(), platform.ID(), platform.ID(), platform.ID()
	for _, s := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO identity.users(id,username,password_hash) VALUES($1,$1,'unused')`, []any{user}},
		{`INSERT INTO shops.stores(id,name) VALUES($1,'نارون')`, []any{store}},
		{`INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, []any{user, store}},
		{`INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{digest(token), user, csrf}},
		{`INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,'fake','fake-account-demo-page','demo-page','connected')`, []any{connection, store}},
	} {
		if _, err = pool.Exec(ctx, s.q, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, q := range []string{`DELETE FROM catalog.post_selection_requests WHERE store_id=$1`, `DELETE FROM catalog.post_selections WHERE store_id=$1`, `DELETE FROM catalog.history WHERE product_id IN (SELECT id FROM catalog.products WHERE store_id=$1)`, `DELETE FROM catalog.codes WHERE store_id=$1`, `DELETE FROM catalog.products WHERE store_id=$1`, `DELETE FROM instagram.connections WHERE store_id=$1`, `DELETE FROM shops.memberships WHERE store_id=$1`, `DELETE FROM shops.stores WHERE id=$1`} {
			if _, e := pool.Exec(ctx, q, store); e != nil {
				t.Error(e)
			}
		}
		pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, user)
		pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, user)
	}()
	auth := NewAuth(pool, "https://shop.example", true)
	auth.Instagram = instagram.Fake{}
	call := func(method, path, body string, want int) []byte {
		r := httptest.NewRequest(method, "/stores/"+store+path, strings.NewReader(body))
		r.Header.Set("Origin", auth.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", "shop_session="+token)
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	call("POST", "/post-selections", `{"postIds":["demo-page-post-01"],"key":"extract-first"}`, 200)
	var list struct {
		Selections []SelectedPost `json:"selections"`
	}
	read := func() SelectedPost {
		t.Helper()
		if e := json.Unmarshal(call("GET", "/post-selections", "", 200), &list); e != nil {
			t.Fatal(e)
		}
		if len(list.Selections) != 1 {
			t.Fatal(list)
		}
		return list.Selections[0]
	}
	if s := read(); s.Status != "pending" {
		t.Fatal(s)
	}
	worker := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool}, Extractor: extraction.ScriptedExtractor{}, Timeout: time.Second}
	if err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	s := read()
	if s.Status != "completed" || s.ProductID == nil || s.Proposal == nil {
		t.Fatalf("missing draft/evidence: %+v", s)
	}
	var detail struct {
		Product Product `json:"product"`
	}
	json.Unmarshal(call("GET", "/products/"+*s.ProductID, "", 200), &detail)
	if detail.Product.Status != "draft" || detail.Product.Name != "مانتو لینن نارون" || len(detail.Product.Variants) != 1 || detail.Product.Variants[0].Price == nil || *detail.Product.Variants[0].Price != 890000 {
		t.Fatal(detail)
	}
	call("POST", "/post-selections", `{"postIds":["demo-page-post-01"],"key":"extract-again"}`, 200)
	if err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	var products struct {
		Products []Product `json:"products"`
	}
	json.Unmarshal(call("GET", "/products", "", 200), &products)
	if len(products.Products) != 1 {
		t.Fatal("duplicate draft", products)
	}
	for _, tc := range []struct {
		name, post, status, reason string
		adapter                    extractorFunc
	}{
		{"timeout", "02", "failed", "timeout", func(ctx context.Context, in extraction.Input) (extraction.Proposal, error) {
			<-ctx.Done()
			return extraction.Proposal{}, ctx.Err()
		}},
		{"invalid", "03", "failed", "invalid_output", func(ctx context.Context, in extraction.Input) (extraction.Proposal, error) {
			p, _ := (extraction.ScriptedExtractor{}).Extract(ctx, in)
			p.Name.State = "invented"
			return p, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call("POST", "/post-selections", `{"postIds":["demo-page-post-`+tc.post+`"],"key":"`+tc.name+`-selection"}`, 200)
			worker := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool, MaxAttempts: 1}, Extractor: tc.adapter, Timeout: 10 * time.Millisecond}
			if err := worker.ProcessOne(ctx); err != nil {
				t.Fatal(err)
			}
			json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
			var failed SelectedPost
			for _, s := range list.Selections {
				if s.ID == "demo-page-post-"+tc.post {
					failed = s
				}
			}
			if failed.Status != tc.status || failed.Reason != tc.reason || failed.ProductID != nil || failed.Attempts != 1 {
				t.Fatalf("unexpected failure %+v", failed)
			}
		})
	}

	json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
	var failedID string
	for _, s := range list.Selections {
		if s.Reason == "timeout" {
			failedID = s.SelectionID
		}
	}
	call("POST", "/post-selections/"+failedID+"/retry", `{}`, 200)
	if err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
	for _, s := range list.Selections {
		if s.SelectionID == failedID && (s.Status != "completed" || s.ProductID == nil) {
			t.Fatal(s)
		}
	}

	var invalidID string
	for _, s := range list.Selections {
		if s.ID == "demo-page-post-03" {
			invalidID = s.SelectionID
		}
	}
	for _, mode := range []string{"foreign_source", "missing_evidence", "negative_price", "refusal", "quota"} {
		t.Run(mode, func(t *testing.T) {
			call("POST", "/post-selections/"+invalidID+"/retry", `{}`, 200)
			adapter := extractorFunc(func(ctx context.Context, in extraction.Input) (extraction.Proposal, error) {
				p, _ := (extraction.ScriptedExtractor{}).Extract(ctx, in)
				switch mode {
				case "foreign_source":
					p.Name.Sources[0].ID = "another-store"
				case "missing_evidence":
					p.Name.Sources[0].Quote = "متن جعلی"
				case "negative_price":
					p.Price = extraction.Field{State: "known", Values: []string{"-1"}, Sources: p.Name.Sources}
				case "refusal":
					return p, extraction.ErrRefusal
				case "quota":
					return p, extraction.ErrBlocked
				}
				return p, nil
			})
			w := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool}, Extractor: adapter}
			if err := w.ProcessOne(ctx); err != nil {
				t.Fatal(err)
			}
			json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
			for _, s := range list.Selections {
				if s.SelectionID == invalidID && (s.ProductID != nil || (s.Status != "failed" && s.Status != "blocked")) {
					t.Fatal(s)
				}
			}
		})
	}
	// A crashed worker's lease can be reclaimed; its stale result is fenced out.
	call("POST", "/post-selections/"+invalidID+"/retry", `{}`, 200)
	q := extraction.PostgresQueue{Pool: pool, Lease: 20 * time.Millisecond}
	stale, err := q.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	fresh, err := (extraction.PostgresQueue{Pool: pool}).Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (extraction.ScriptedExtractor{}).Extract(ctx, fresh.Input)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Complete(ctx, stale, proposal); !errors.Is(err, extraction.ErrLease) {
		t.Fatal("stale lease accepted", err)
	}
	proposal.Attributes["جنس"] = extraction.Field{State: "conflict", Values: []string{"پارچه‌ای", "طبیعی"}, Sources: []extraction.Source{{ID: fresh.SourceID, Quote: "پارچه‌ای"}, {ID: fresh.SourceID, Quote: "طبیعی"}}}
	if err = q.Complete(ctx, fresh, proposal); err != nil {
		t.Fatal(err)
	}
	if err = q.Complete(ctx, fresh, proposal); !errors.Is(err, extraction.ErrLease) {
		t.Fatal("completed twice", err)
	}
	json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
	for _, s := range list.Selections {
		if s.SelectionID == invalidID {
			if s.Proposal.Price.State != "unknown" || s.Proposal.Attributes["جنس"].State != "conflict" || s.ProductID == nil {
				t.Fatal(s)
			}
			json.Unmarshal(call("GET", "/products/"+*s.ProductID, "", 200), &detail)
			if detail.Product.Status != "draft" || detail.Product.Variants[0].Price != nil || detail.Product.Variants[0].Attributes["جنس"] != "" {
				t.Fatal("guessed unknown/conflicting field", detail)
			}
			call("POST", "/post-selections/"+invalidID+"/retry", `{}`, 409)
		}
	}

	// A queue failure must roll back selection too, so a client can safely retry.
	auth.ExtractionQueue = brokenExtractionQueue{Queue: extraction.PostgresQueue{Pool: pool}}
	call("POST", "/post-selections", `{"postIds":["demo-page-post-04"],"key":"atomic-enqueue"}`, 503)
	json.Unmarshal(call("GET", "/post-selections", "", 200), &list)
	if len(list.Selections) != 3 {
		t.Fatal("selection committed without durable job", list)
	}
	auth.ExtractionQueue = extraction.PostgresQueue{Pool: pool}
	call("POST", "/post-selections", `{"postIds":["demo-page-post-04"],"key":"atomic-enqueue"}`, 200)
	if err = worker.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}

}
