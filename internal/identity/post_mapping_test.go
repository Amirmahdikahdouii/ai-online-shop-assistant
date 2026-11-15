package identity

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/extraction"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
)

func TestPostMappingLifecycle(t *testing.T) {
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

	selection := func(post, key string) string {
		var result struct{ Selections []SelectedPost }
		json.Unmarshal(call("POST", base+"/post-selections", `{"postIds":["`+post+`"],"key":"`+key+`"}`, 0, 200).Body.Bytes(), &result)
		if len(result.Selections) != 1 {
			t.Fatal(result)
		}
		return result.Selections[0].SelectionID
	}
	id := selection("demo-page-post-01", "mapping-first")
	id2 := selection("demo-page-post-02", "mapping-second")
	product := func(who int) Product {
		var result struct{ Product Product }
		json.Unmarshal(call("POST", "/stores/"+stores[who]+"/products", `{"name":"پیراهن","variants":[{"name":"آبی","code":null,"price":null,"attributes":{}}]}`, who, 201).Body.Bytes(), &result)
		return result.Product
	}
	p, p2, foreign := product(0), product(0), product(1)
	defer func() {
		for _, store := range stores {
			for _, q := range []string{`DELETE FROM catalog.post_selections WHERE store_id=$1`, `DELETE FROM catalog.history WHERE product_id IN (SELECT id FROM catalog.products WHERE store_id=$1)`, `DELETE FROM catalog.products WHERE store_id=$1`} {
				if _, e := pool.Exec(ctx, q, store); e != nil {
					t.Error(e)
				}
			}
		}
	}()
	path := base + "/post-selections/" + id + "/mapping"
	call("GET", path, "", 1, 404)
	call("GET", path, "", 0, 200)
	body := `{"revision":0,"notProduct":false,"targets":[{"productId":"` + p.ID + `","variantId":"` + p.Variants[0].ID + `"},{"productId":"` + p2.ID + `","variantId":""}]}`
	saved := call("POST", path, body, 0, 200)
	if !strings.Contains(saved.Body.String(), `"revision":1`) {
		t.Fatal(saved.Body.String())
	}
	// Repeated save and fresh-process reads preserve one decision/version.
	call("POST", path, body, 0, 200)
	auth = NewAuth(pool, "https://shop.example", true)
	for range 3 {
		w := call("GET", path, "", 0, 200)
		var result struct {
			Mapping struct {
				Revision int
				Targets  []struct{ ProductID string }
			}
			History []any
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		if result.Mapping.Revision != 1 || len(result.Mapping.Targets) != 2 || len(result.History) != 1 {
			t.Fatal(w.Body.String())
		}
	}
	// One catalog product can be reused by another post.
	call("POST", base+"/post-selections/"+id2+"/mapping", `{"revision":0,"targets":[{"productId":"`+p.ID+`","variantId":""}]}`, 0, 200)
	call("POST", path, `{"revision":1,"targets":[{"productId":"`+foreign.ID+`","variantId":""}]}`, 0, 422)
	call("POST", path, `{"revision":1,"targets":[{"productId":"`+p.ID+`","variantId":"`+p2.Variants[0].ID+`"}]}`, 0, 422)
	call("POST", path, `{"revision":0,"notProduct":true,"targets":[]}`, 0, 409)
	corrected := call("POST", path, `{"revision":1,"notProduct":true,"targets":[]}`, 0, 200)
	if !strings.Contains(corrected.Body.String(), `"revision":2`) {
		t.Fatal(corrected.Body.String())
	}

	// Background extraction can finish after a manager's answer without replacing it.
	worker := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool}, Extractor: extraction.ScriptedExtractor{}}
	for range 2 {
		if e := worker.ProcessOne(ctx); e != nil {
			t.Fatal(e)
		}
	}
	w := call("GET", path, "", 0, 200)
	var result struct {
		Mapping struct {
			Revision   int
			NotProduct bool
		}
		History []struct{ ActorID, Source, ChangedAt string }
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Mapping.Revision != 2 || !result.Mapping.NotProduct || len(result.History) != 2 || result.History[1].ActorID != users[0] || result.History[1].Source != "manager" || result.History[1].ChangedAt == "" {
		t.Fatal(w.Body.String())
	}
	call("POST", path, `{"revision":2,"notProduct":true,"targets":[{"productId":"`+p.ID+`","variantId":""}]}`, 0, 422)
}
