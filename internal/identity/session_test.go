package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

func TestAuthOriginAndCookie(t *testing.T) {
	a := NewAuth(nil, "https://shop.example", true)
	for _, origin := range []string{"", "https://evil.example"} {
		r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.cookie(w, "opaque", 604800)
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 604800 {
		t.Fatal(c)
	}
}

func TestSessionsPostgres(t *testing.T) {
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
	name := "sess_" + strings.ToLower(platform.ID())
	password := "session-password-1234"
	defer pool.Exec(ctx, `DELETE FROM identity.users WHERE username=$1`, name)
	defer pool.Exec(ctx, `DELETE FROM identity.login_attempts WHERE bucket=$1`, digest(name))
	call := func(h http.Handler, method, path, body, token, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "shop_session", Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	credentials, _ := json.Marshal(map[string]string{"username": name, "password": password})
	if w := call(RegisterHandler(pool), "POST", "/auth/register", string(credentials), "", ""); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	a := NewAuth(pool, "https://shop.example", true)
	if w := call(a, "GET", "/me", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	wrong, _ := json.Marshal(map[string]string{"username": name, "password": "wrong"})
	if w := call(a, "POST", "/auth/login", string(wrong), "", ""); w.Code != 401 || strings.Contains(w.Body.String(), name) {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call(a, "POST", "/auth/login", string(credentials), "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	token := w.Result().Cookies()[0].Value
	var result struct {
		CSRF string `json:"csrfToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	var stored string
	if err = pool.QueryRow(ctx, `SELECT token_hash FROM identity.sessions WHERE token_hash=$1`, digest(token)).Scan(&stored); err != nil || stored == token {
		t.Fatal("opaque token must only be stored hashed", err)
	}
	// New handler simulates process restart; no provider/connection is required.
	a = NewAuth(pool, "https://shop.example", true)
	if w = call(a, "GET", "/me", "", token, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM identity.login_attempts WHERE bucket=$1`, digest(name)).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("successful login should clear account attempts", remaining, err)
	}
	if w = call(a, "POST", "/auth/logout", "{}", token, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = call(a, "POST", "/auth/logout", "{}", token, result.CSRF); w.Code != 200 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(a, "GET", "/me", "", token, ""); w.Code != 401 {
		t.Fatal("revoked session", w.Code)
	}
	w = call(a, "POST", "/auth/login", string(credentials), "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	token = w.Result().Cookies()[0].Value
	if _, err = pool.Exec(ctx, `UPDATE identity.sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if w = call(a, "GET", "/me", "", token, ""); w.Code != 401 {
		t.Fatal("expired session", w.Code)
	}
	for i := 0; i < 12; i++ {
		w = call(a, "POST", "/auth/login", string(wrong), "", "")
	}
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("rate limit", w.Code)
	}
}
