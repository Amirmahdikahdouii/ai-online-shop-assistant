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

type controlledPosts struct {
	instagram.Fake
	mode string
}

func (f controlledPosts) ListPosts(ctx context.Context, id instagram.Identity, cursor string) (instagram.PostPage, error) {
	if f.mode == "empty" {
		return instagram.PostPage{Posts: []instagram.Post{}}, nil
	}
	if f.mode == "error" {
		return instagram.PostPage{}, context.DeadlineExceeded
	}
	return f.Fake.ListPosts(ctx, id, cursor)
}

func TestPostSelectionLifecycle(t *testing.T) {
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
	users := []string{platform.ID(), platform.ID()}
	stores := []string{platform.ID(), platform.ID()}
	tokens := []string{platform.ID(), platform.ID()}
	csrf := platform.ID()
	defer func() {
		for i, u := range users {
			for _, q := range []string{`DELETE FROM catalog.post_selection_requests WHERE store_id=$1`, `DELETE FROM catalog.post_selections WHERE store_id=$1`, `DELETE FROM instagram.connections WHERE store_id=$1`, `DELETE FROM shops.memberships WHERE store_id=$1`, `DELETE FROM shops.stores WHERE id=$1`} {
				if _, e := pool.Exec(ctx, q, stores[i]); e != nil {
					t.Error(e)
				}
			}
			_, _ = pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, u)
			_, _ = pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, u)
		}
	}()
	for i, u := range users {
		for _, s := range []struct {
			q    string
			args []any
		}{
			{`INSERT INTO identity.users(id,username,password_hash) VALUES($1,$2,'unused')`, []any{u, "posts_" + strings.ToLower(u)}},
			{`INSERT INTO shops.stores(id,name) VALUES($1,'پوشاک نارون')`, []any{stores[i]}},
			{`INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, []any{u, stores[i]}},
			{`INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{digest(tokens[i]), u, csrf}},
		} {
			if _, err = pool.Exec(ctx, s.q, s.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	auth := NewAuth(pool, "https://shop.example", true)
	auth.Instagram = instagram.Fake{}
	call := func(method, path, body string, who, want int) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if who >= 0 {
			r.Header.Set("Cookie", "shop_session="+tokens[who])
		}
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	base := "/stores/" + stores[0]
	call("GET", base+"/posts", "", -1, 401)
	call("GET", base+"/posts", "", 1, 404)
	call("GET", base+"/posts", "", 0, 409)
	for i, page := range []string{"demo-page", "other-page"} {
		if _, err = pool.Exec(ctx, `INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,'fake',$3,$4,'connected')`, platform.ID(), stores[i], "fake-account-"+page, page); err != nil {
			t.Fatal(err)
		}
	}
	var page struct {
		Posts []struct {
			instagram.Post
			Selected bool   `json:"selected"`
			Status   string `json:"status"`
		}
		NextCursor string `json:"nextCursor"`
	}
	json.Unmarshal(call("GET", base+"/posts", "", 0, 200).Body.Bytes(), &page)
	if len(page.Posts) != 6 || page.NextCursor == "" || page.Posts[0].Caption == "" || page.Posts[0].PublishedAt.IsZero() {
		t.Fatal(page)
	}
	call("GET", base+"/posts?cursor=bad", "", 0, 422)
	body := `{"postIds":["demo-page-post-01","demo-page-post-07","demo-page-post-01"],"key":"selection-one"}`
	first := call("POST", base+"/post-selections", body, 0, 200)
	var selected struct {
		Selections []SelectedPost `json:"selections"`
	}
	json.Unmarshal(first.Body.Bytes(), &selected)
	if len(selected.Selections) != 2 || selected.Selections[0].Status != "pending" || selected.Selections[0].SelectionID == "" {
		t.Fatal(first.Body.String())
	}
	again := call("POST", base+"/post-selections", body, 0, 200)
	var a, b any
	json.Unmarshal(first.Body.Bytes(), &a)
	json.Unmarshal(again.Body.Bytes(), &b)
	// requestId is per HTTP request; compare the persisted selection result only.
	aa, _ := json.Marshal(a.(map[string]any)["selections"])
	bb, _ := json.Marshal(b.(map[string]any)["selections"])
	if string(aa) != string(bb) {
		t.Fatal("replay changed result")
	}
	call("POST", base+"/post-selections", `{"postIds":["demo-page-post-02"],"key":"selection-one"}`, 0, 409)
	call("POST", base+"/post-selections", `{"postIds":["other-page-post-01"],"key":"foreign-post"}`, 0, 422)
	call("POST", base+"/post-selections", `{"postIds":["demo-page-post-02","missing"],"key":"atomic-invalid"}`, 0, 422)
	call("POST", base+"/post-selections", body, 1, 404)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			call("POST", base+"/post-selections", `{"postIds":["demo-page-post-01","demo-page-post-07"],"key":"parallel-key"}`, 0, 200)
		}()
	}
	wg.Wait()
	fresh := NewAuth(pool, "https://shop.example", true)
	fresh.Instagram = instagram.Fake{}
	auth = fresh
	json.Unmarshal(call("GET", base+"/posts?cursor=6", "", 0, 200).Body.Bytes(), &page)
	if len(page.Posts) != 2 || !page.Posts[0].Selected || page.Posts[0].Status != "pending" {
		t.Fatal(page)
	}
	call("GET", base+"/post-selections", "", 1, 404)
	listed := call("GET", base+"/post-selections", "", 0, 200)
	json.Unmarshal(listed.Body.Bytes(), &selected)
	if len(selected.Selections) != 2 {
		t.Fatal(listed.Body.String())
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM catalog.post_selections WHERE store_id=$1`, stores[0]).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	auth.Instagram = controlledPosts{mode: "empty"}
	json.Unmarshal(call("GET", base+"/posts", "", 0, 200).Body.Bytes(), &page)
	if len(page.Posts) != 0 {
		t.Fatal(page)
	}
	auth.Instagram = controlledPosts{mode: "error"}
	call("GET", base+"/posts", "", 0, 503)
	// Stored selections remain readable through provider failure or disconnect.
	call("GET", base+"/post-selections", "", 0, 200)
	auth.Instagram = nil
	call("GET", base+"/posts", "", 0, 503)
	call("GET", base+"/post-selections", "", 0, 200)
	auth.Instagram = instagram.Fake{}
	call("POST", base+"/post-selections", `{"postIds":["demo-page-post-03"],"key":"single-select"}`, 0, 200)
	var events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM processing.events WHERE store_id=$1`, stores[0]).Scan(&events); err != nil || events != 0 {
		t.Fatal("post selection imported messages", events, err)
	}
}
