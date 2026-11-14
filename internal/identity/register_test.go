package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"shopassistant/internal/platform"
)

// This boundary test requires a dedicated real PostgreSQL database. Never use a
// live database: migrations and account creation are intentional test effects.
func TestRegisterPostgres(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set REGISTRATION_TEST_DATABASE_URL to a dedicated PostgreSQL database")
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
	handler := RegisterHandler(pool)
	name := "test_" + strings.ToLower(platform.ID())
	defer pool.Exec(ctx, "DELETE FROM identity.users WHERE username=$1", name)
	password := "a-password-with-۱۲۳۴-and-spaces "
	body, _ := json.Marshal(map[string]string{"username": " " + strings.ToUpper(name) + " ", "password": password})
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/auth/register", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call(); w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	var phc string
	if err := pool.QueryRow(ctx, "SELECT password_hash FROM identity.users WHERE username=$1", name).Scan(&phc); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		t.Fatalf("invalid password encoding")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatal(err)
	}
	actual := base64.RawStdEncoding.EncodeToString(argon2.IDKey([]byte(password), salt, 3, 65536, 1, 32))
	if actual != parts[5] {
		t.Fatal("stored hash does not verify the exact password")
	}
	if w := call(); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
}

func TestRejectMalformedUnicodeAtAPI(t *testing.T) {
	handler := RegisterHandler(nil) // Invalid input must never reach storage/hash.
	for _, body := range []string{`{"username":"valid","password":"long-password-\ud800"}`, `{"username":"valid","password":"long-password-\udc00"}`, "{\"password\":\"invalid-\xff\"}"} {
		r := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("malformed Unicode: got %d", w.Code)
		}
	}
	for _, body := range []string{`{"password":"valid-\ud83d\ude00"}`, `{"password":"literal-\\ud800"}`, `{"password":"فارسی"}`} {
		if !validUnicodeJSON([]byte(body)) {
			t.Fatal("rejected valid Unicode")
		}
	}
}
