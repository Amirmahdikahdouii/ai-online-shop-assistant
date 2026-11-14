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
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
)

func TestConnectionLifecycle(t *testing.T) {
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
	users := []string{platform.ID(), platform.ID()}
	stores := []string{platform.ID(), platform.ID()}
	tokens := []string{platform.ID(), platform.ID()}
	defer func() {
		for i := range users {
			pool.Exec(ctx, `DELETE FROM instagram.connection_attempts WHERE store_id=$1`, stores[i])
			pool.Exec(ctx, `DELETE FROM instagram.connections WHERE store_id=$1`, stores[i])
			pool.Exec(ctx, `DELETE FROM shops.memberships WHERE store_id=$1`, stores[i])
			pool.Exec(ctx, `DELETE FROM shops.stores WHERE id=$1`, stores[i])
			pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, users[i])
			pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, users[i])
		}
	}()
	for i := range users {
		_, err = pool.Exec(ctx, `INSERT INTO identity.users(id,username,password_hash) VALUES($1,$1,'unused')`, users[i])
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO shops.stores(id,name) VALUES($1,'test')`, stores[i])
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, users[i], stores[i])
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'csrf',clock_timestamp()+interval '1 hour')`, digest(tokens[i]), users[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	a := NewAuth(pool, "https://shop.example", true)
	a.Instagram = instagram.Fake{}
	call := func(user, store int, action, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		method := "POST"
		if action == "status" {
			method = "GET"
		}
		r := httptest.NewRequest(method, "/stores/"+stores[store]+"/connection/"+action, strings.NewReader(body))
		r.Header.Set("Origin", a.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Cookie", "shop_session="+tokens[user])
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%s: got %d want %d: %s", action, w.Code, want, w.Body.String())
		}
		return w
	}
	start := func(i int) string {
		t.Helper()
		var result struct {
			State string `json:"state"`
		}
		json.Unmarshal(call(i, i, "start", "{}", 200).Body.Bytes(), &result)
		return result.State
	}
	callback := func(state, code string) string {
		b, _ := json.Marshal(map[string]string{"state": state, "code": code})
		return string(b)
	}
	state := start(0)
	call(1, 0, "callback", callback(state, "demo-page"), 404)
	call(0, 0, "callback", callback("invalid", "demo-page"), 400)
	call(0, 0, "callback", callback(state, "invalid"), 400)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); call(0, 0, "callback", callback(state, "demo-page"), 200) }()
	}
	wg.Wait()
	status := func() Connection {
		t.Helper()
		var result struct {
			Connection Connection `json:"connection"`
		}
		if err := json.Unmarshal(call(0, 0, "status", "", 200).Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Connection
	}
	before := status()
	call(0, 0, "disconnect", "{}", 200)
	crm := httptest.NewRequest("GET", "/me", nil)
	crm.Header.Set("Cookie", "shop_session="+tokens[0])
	crmResponse := httptest.NewRecorder()
	a.ServeHTTP(crmResponse, crm)
	if crmResponse.Code != 200 || !strings.Contains(crmResponse.Body.String(), stores[0]) {
		t.Fatal("disconnect lost CRM access", crmResponse.Body.String())
	}
	call(0, 0, "callback", callback(state, "demo-page"), 400)
	if w := call(0, 0, "status", "", 200); !strings.Contains(w.Body.String(), "disconnected") {
		t.Fatal(w.Body.String())
	}
	call(0, 0, "callback", callback(start(0), "demo-page"), 200)
	if after := status(); before != after {
		t.Fatal("reconnect changed identity or receiving start", before, after)
	}
	call(0, 0, "callback", callback(start(0), "other-page"), 409)
	call(1, 1, "callback", callback(start(1), "demo-page"), 409)
	call(0, 0, "disconnect", "{}", 200)
	call(1, 1, "callback", callback(start(1), "demo-page"), 200)
	call(0, 0, "callback", callback(start(0), "demo-page"), 409)
	call(1, 1, "disconnect", "{}", 200)
	call(0, 0, "callback", callback(start(0), "demo-page"), 200)
	expired := start(1)
	pool.Exec(ctx, `UPDATE instagram.connection_attempts SET expires_at=clock_timestamp()-interval '1 second' WHERE state_hash=$1`, digest(expired))
	call(1, 1, "callback", callback(expired, "other-page"), 400)
	// Reconstructing the handler models API restart; all state is in PostgreSQL.
	a = NewAuth(pool, "https://shop.example", true)
	a.Instagram = instagram.Fake{}
	if after := status(); before != after {
		t.Fatal("restart lost state")
	}
	a.Instagram = nil
	call(0, 0, "start", "{}", 503)
}
