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

func TestCatalogValidation(t *testing.T) {
	price := int64(-1)
	for _, p := range []Product{{}, {Name: "x", Variants: []Variant{{Name: "x", Price: &price}}}, {Name: "x", Variants: []Variant{{Name: "x", Attributes: map[string]string{"color": ""}}}}} {
		if len(validateProduct(&p)) == 0 {
			t.Fatal("invalid product accepted", p)
		}
	}
	p := Product{Name: " محصول ", Variants: []Variant{{Name: "ساده"}}}
	if len(validateProduct(&p)) != 0 || p.Variants[0].Price != nil || p.Name != "محصول" {
		t.Fatal(p)
	}
}

func TestCatalogLifecycle(t *testing.T) {
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
		for i, user := range users {
			_, _ = pool.Exec(ctx, `DELETE FROM catalog.history WHERE product_id IN (SELECT id FROM catalog.products WHERE store_id=$1)`, stores[i])
			_, _ = pool.Exec(ctx, `DELETE FROM catalog.codes WHERE store_id=$1`, stores[i])
			_, _ = pool.Exec(ctx, `DELETE FROM catalog.products WHERE store_id=$1`, stores[i])
			_, _ = pool.Exec(ctx, `DELETE FROM shops.memberships WHERE user_id=$1`, user)
			_, _ = pool.Exec(ctx, `DELETE FROM shops.stores WHERE id=$1`, stores[i])
			_, _ = pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, user)
			_, _ = pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, user)
		}
	}()
	for i, user := range users {
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO identity.users(id,username,password_hash) VALUES($1,$2,'unused')`, []any{user, "cat_" + strings.ToLower(user)}},
			{`INSERT INTO shops.stores(id,name) VALUES($1,'catalog test')`, []any{stores[i]}},
			{`INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, []any{user, stores[i]}},
			{`INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{digest(tokens[i]), user, csrf}},
		} {
			if _, err = pool.Exec(ctx, statement.sql, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	auth := NewAuth(pool, "https://shop.example", true)
	call := func(method, path string, body any, who int, expected int) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if who >= 0 {
			r.Header.Set("Cookie", "shop_session="+tokens[who])
		}
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, r)
		if expected != 0 && w.Code != expected {
			t.Errorf("%s %s: got %d want %d: %s", method, path, w.Code, expected, w.Body.String())
		}
		return w
	}
	base := "/stores/" + stores[0] + "/products"
	code := "SKU-1"
	input := func(c *string) map[string]any {
		return map[string]any{"name": "پیراهن", "variants": []Variant{{Name: "آبی متوسط", Code: c, Attributes: map[string]string{"رنگ": "آبی"}}, {Name: "قرمز", Price: func() *int64 { v := int64(120000); return &v }()}}}
	}
	decode := func(w *httptest.ResponseRecorder) Product {
		var data struct {
			Product Product `json:"product"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data.Product
	}
	call("GET", base, nil, -1, 401)
	call("GET", base, nil, 1, 404)
	p := decode(call("POST", base, input(&code), 0, 201))
	path := base + "/" + p.ID
	if p.Status != "draft" || p.Variants[0].Price != nil || *p.Variants[1].Price != 120000 {
		t.Fatal(p)
	}
	call("POST", base, input(&code), 0, 409)
	call("POST", "/stores/"+stores[1]+"/products", input(&code), 1, 201)
	call("GET", path, nil, 1, 404)
	call("POST", path+"/approve", map[string]int{"revision": 1}, 1, 404)
	call("GET", "/stores/"+stores[1]+"/products/"+p.ID, nil, 1, 404)
	call("POST", path+"/approve", map[string]int{"revision": 0}, 0, 409)
	p = decode(call("POST", path+"/approve", map[string]int{"revision": 1}, 0, 200))
	if p.Status != "approved" || p.Variants[0].Price != nil {
		t.Fatal(p)
	}
	p.Name = "پیراهن جدید"
	edit := map[string]any{"revision": p.Revision, "name": p.Name, "variants": p.Variants}
	p = decode(call("POST", path, edit, 0, 200))
	call("POST", path, edit, 0, 409)
	if p.Revision != 3 {
		t.Fatal(p)
	}
	w := call("GET", path, nil, 0, 200)
	var detail struct {
		History []struct {
			Revision int
			Snapshot Product
		} `json:"history"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail.History) != 3 || detail.History[0].Snapshot.Name != "پیراهن" {
		t.Fatal(w.Body.String())
	}
	for _, q := range []string{"q=SKU-1", "q=آبی", "q=پیراهن", "status=approved"} {
		w = call("GET", base+"?"+q, nil, 0, 200)
		if !strings.Contains(w.Body.String(), p.ID) {
			t.Fatal(q, w.Body.String())
		}
	}
	p = decode(call("POST", path+"/archive", map[string]int{"revision": p.Revision}, 0, 200))
	if p.Status != "archived" {
		t.Fatal(p)
	}
	call("POST", base, input(&code), 0, 409)
	call("POST", path, edit, 0, 409)
	call("POST", base, map[string]any{"name": "", "variants": []Variant{}}, 0, 422)
	call("POST", base, input(nil), 0, 201)
	// Approval and correction using the same revision serialize: one wins,
	// the loser cannot overwrite content or add an audit event.
	raceProduct := decode(call("POST", base, input(nil), 0, 201))
	racePath := base + "/" + raceProduct.ID
	raceEdit := map[string]any{"revision": 1, "name": "اصلاح هم‌زمان", "variants": raceProduct.Variants}
	start := make(chan struct{})
	outcomes := make(chan int, 2)
	go func() { <-start; outcomes <- call("POST", racePath, raceEdit, 0, 0).Code }()
	go func() {
		<-start
		outcomes <- call("POST", racePath+"/approve", map[string]int{"revision": 1}, 0, 0).Code
	}()
	close(start)
	a, b := <-outcomes, <-outcomes
	if !((a == 200 && b == 409) || (a == 409 && b == 200)) {
		t.Fatalf("concurrent edit/approval: %d %d", a, b)
	}
	var audited struct {
		Product Product
		History []struct {
			ActorID   string
			ChangedAt string
			Snapshot  Product
		}
	}
	json.Unmarshal(call("GET", racePath, nil, 0, 200).Body.Bytes(), &audited)
	if audited.Product.Revision != 2 || len(audited.History) != 2 || audited.History[1].ActorID != users[0] || audited.History[1].ChangedAt == "" {
		t.Fatalf("missing atomic revision audit: %+v", audited)
	}
	// Concurrent claims for a new code must leave exactly one complete product.
	concurrent := "RACE"
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- call("POST", base, input(&concurrent), 0, 0).Code }()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for code := range results {
		counts[code]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
	for range 22 {
		call("POST", base, input(nil), 0, 201)
	}
	w = call("GET", base, nil, 0, 200)
	var page struct {
		Products   []Product
		NextCursor string
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Products) != 20 || page.NextCursor == "" {
		t.Fatal(w.Body.String())
	}
	first := append([]Product(nil), page.Products...)
	w = call("GET", base+"?cursor="+page.NextCursor, nil, 0, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Products) == 0 {
		t.Fatal("missing second page")
	}
	for _, a := range first {
		for _, b := range page.Products {
			if a.ID == b.ID {
				t.Fatal("duplicate page item")
			}
		}
	}
	// A fresh Auth instance reads persisted data, without process-local catalog state.
	auth = NewAuth(pool, "https://shop.example", true)
	call("GET", path, nil, 0, 200)
	large := Product{Name: "مجموعهٔ فارسی"}
	for range 20 {
		large.Variants = append(large.Variants, Variant{Name: "تنوع فارسی", Attributes: map[string]string{"توضیحات": strings.Repeat("الف", 60)}})
	}
	raw, _ := json.Marshal(large)
	if len(raw) <= 4096 {
		t.Fatal("fixture must exceed old body limit")
	}
	created := decode(call("POST", base, map[string]any{"name": large.Name, "variants": large.Variants}, 0, 201))
	updated := decode(call("POST", base+"/"+created.ID, map[string]any{"name": created.Name, "variants": created.Variants, "revision": created.Revision}, 0, 200))
	if len(updated.Variants) != 20 || updated.Variants[19].Attributes["توضیحات"] != large.Variants[19].Attributes["توضیحات"] {
		t.Fatal("large product did not round trip")
	}
}
