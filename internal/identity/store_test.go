package identity

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

func TestStoreAtomicityAndIsolation(t *testing.T) {
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
	if err := platform.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	users := []string{platform.ID(), platform.ID()}
	tokens := []string{platform.ID(), platform.ID()}
	csrf := platform.ID()
	defer func() {
		for _, id := range users {
			_, _ = pool.Exec(ctx, `WITH removed AS (DELETE FROM shops.memberships WHERE user_id=$1 RETURNING store_id) DELETE FROM shops.stores WHERE id IN (SELECT store_id FROM removed)`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, id)
		}
	}()
	for i, id := range users {
		if _, err := pool.Exec(ctx, `INSERT INTO identity.users(id,username,password_hash) VALUES($1,$2,'unused')`, id, "store_"+strings.ToLower(id)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, digest(tokens[i]), id, csrf); err != nil {
			t.Fatal(err)
		}
	}
	auth := NewAuth(pool, "https://shop.example", true)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Cookie", "shop_session="+token)
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, r)
		return w
	}
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM shops.stores`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	responses := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- call("POST", "/stores", `{"name":"پوشاک نارون"}`, tokens[0])
		}()
	}
	wg.Wait()
	close(responses)
	successes, conflicts := 0, 0
	var first struct {
		Store Store `json:"store"`
	}
	for w := range responses {
		switch w.Code {
		case 201:
			successes++
			if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
				t.Fatal(err)
			}
		case 409:
			conflicts++
		default:
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(successes, conflicts)
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM shops.stores`).Scan(&after); err != nil || after != before+1 {
		t.Fatal("failed create left an orphan", before, after, err)
	}
	if w := call("GET", "/stores/"+first.Store.ID, "", tokens[1]); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("GET", "/stores/"+first.Store.ID, "", tokens[0]); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, users[1], first.Store.ID); err == nil {
		t.Fatal("second manager allowed")
	}
	if w := call("POST", "/stores", `{"name":"کیف آبان"}`, tokens[1]); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("GET", "/me?storeId="+first.Store.ID, "", tokens[1]); w.Code != 200 || strings.Contains(w.Body.String(), first.Store.ID) {
		t.Fatal("client ID bypassed session scope", w.Code, w.Body.String())
	}
}
