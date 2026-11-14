package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
	"shopassistant/internal/processing"
)

func normalizeUsername(s string) string {
	return strings.NewReplacer("ي", "ی", "ى", "ی", "ك", "ک").Replace(cases.Fold().String(norm.NFKC.String(strings.TrimSpace(s))))
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// Auth owns sessions independently of any store or external connection.
type Auth struct {
	Pool       *pgxpool.Pool
	Origin     string
	Secure     bool
	slots      chan struct{}
	Instagram  instagram.Adapter
	Processing *processing.Service
}

func NewAuth(pool *pgxpool.Pool, origin string, secure bool) *Auth {
	return &Auth{Pool: pool, Origin: origin, Secure: secure, slots: make(chan struct{}, 2)}
}

func (a *Auth) cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "shop_session", Value: token, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode, MaxAge: age})
}

func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	id := platform.ID()
	w.Header().Set("X-Request-ID", id)
	reply := func(status int, value map[string]any) {
		value["requestId"] = id
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(value)
	}
	fail := func(status int, code string) { reply(status, map[string]any{"code": code, "fieldErrors": nil}) }
	if r.Method == "POST" && (r.Header.Get("Origin") != a.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
		fail(403, "origin_rejected")
		return
	}
	if r.URL.Path == "/auth/login" && r.Method == "POST" {
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			fail(415, "json_required")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			fail(413, "body_too_large")
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.DisallowUnknownFields()
		if !validUnicodeJSON(body) || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			fail(400, "invalid_json")
			return
		}
		name := normalizeUsername(input.Username)
		// Bound expensive verification and arbitrary username-bucket creation across
		// replicas. No client-supplied forwarding address is trusted as an identity.
		var total int
		err = a.Pool.QueryRow(r.Context(), `INSERT INTO identity.login_attempts(bucket,attempts,expires_at) VALUES('global',1,clock_timestamp()+interval '1 minute') ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN identity.login_attempts.expires_at<=clock_timestamp() THEN 1 ELSE identity.login_attempts.attempts+1 END, expires_at=CASE WHEN identity.login_attempts.expires_at<=clock_timestamp() THEN EXCLUDED.expires_at ELSE identity.login_attempts.expires_at END RETURNING attempts`).Scan(&total)
		if err != nil {
			fail(503, "auth_unavailable")
			return
		}
		if total > 60 {
			w.Header().Set("Retry-After", "60")
			fail(429, "login_limited")
			return
		}
		if _, err = a.Pool.Exec(r.Context(), `DELETE FROM identity.login_attempts WHERE expires_at<=clock_timestamp(); DELETE FROM identity.sessions WHERE expires_at<=clock_timestamp()`); err != nil {
			fail(503, "auth_unavailable")
			return
		}
		// Shared PostgreSQL buckets survive restarts and apply across API replicas.
		var attempts int
		err = a.Pool.QueryRow(r.Context(), `INSERT INTO identity.login_attempts(bucket,attempts,expires_at) VALUES($1,1,clock_timestamp()+interval '15 minutes') ON CONFLICT(bucket) DO UPDATE SET attempts=CASE WHEN identity.login_attempts.expires_at<=clock_timestamp() THEN 1 ELSE identity.login_attempts.attempts+1 END, expires_at=CASE WHEN identity.login_attempts.expires_at<=clock_timestamp() THEN EXCLUDED.expires_at ELSE identity.login_attempts.expires_at END RETURNING attempts`, digest(name)).Scan(&attempts)
		if err != nil {
			fail(503, "auth_unavailable")
			return
		}
		if attempts > 10 {
			w.Header().Set("Retry-After", "900")
			fail(429, "login_limited")
			return
		}
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		default:
			w.Header().Set("Retry-After", "2")
			fail(429, "login_limited")
			return
		}
		var userID, hash string
		err = a.Pool.QueryRow(r.Context(), `SELECT id,password_hash FROM identity.users WHERE username=$1`, name).Scan(&userID, &hash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			fail(503, "auth_unavailable")
			return
		}
		// Missing users perform the same expensive hash as an existing account.
		salt := make([]byte, 16)
		expected := make([]byte, 32)
		if hash != "" {
			parts := strings.Split(hash, "$")
			if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=1" {
				fail(503, "auth_unavailable")
				return
			}
			salt, err = base64.RawStdEncoding.DecodeString(parts[4])
			if err != nil {
				fail(503, "auth_unavailable")
				return
			}
			expected, err = base64.RawStdEncoding.DecodeString(parts[5])
			if err != nil {
				fail(503, "auth_unavailable")
				return
			}
		}
		actual := argon2.IDKey([]byte(input.Password), salt, 3, 65536, 1, 32)
		if subtle.ConstantTimeCompare(actual, expected) != 1 || userID == "" {
			fail(401, "invalid_credentials")
			return
		}
		token, csrf := platform.ID()+platform.ID(), platform.ID()
		_, err = a.Pool.Exec(r.Context(), `INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '7 days')`, digest(token), userID, csrf)
		if err != nil {
			fail(503, "auth_unavailable")
			return
		}
		// Successful authentication starts a fresh account-attempt window; the
		// installation-wide resource limit still counts all login traffic.
		if _, err = a.Pool.Exec(r.Context(), `DELETE FROM identity.login_attempts WHERE bucket=$1`, digest(name)); err != nil {
			fail(503, "auth_unavailable")
			return
		}
		a.cookie(w, token, 7*24*60*60)
		reply(200, map[string]any{"user": map[string]string{"id": userID, "username": name}, "csrfToken": csrf})
		return
	}
	cookie, err := r.Cookie("shop_session")
	if err != nil {
		fail(401, "session_required")
		return
	}
	var userID, name, csrf string
	err = a.Pool.QueryRow(r.Context(), `SELECT u.id,u.username,s.csrf_token FROM identity.sessions s JOIN identity.users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>clock_timestamp()`, digest(cookie.Value)).Scan(&userID, &name, &csrf)
	if errors.Is(err, pgx.ErrNoRows) {
		a.cookie(w, "", -1)
		fail(401, "session_expired")
		return
	}
	if err != nil {
		fail(503, "auth_unavailable")
		return
	}
	if r.URL.Path == "/auth/logout" && r.Method == "POST" {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
			fail(403, "csrf_rejected")
			return
		}
		if _, err = a.Pool.Exec(r.Context(), `DELETE FROM identity.sessions WHERE token_hash=$1`, digest(cookie.Value)); err != nil {
			fail(503, "auth_unavailable")
			return
		}
		a.cookie(w, "", -1)
		reply(200, map[string]any{"loggedOut": true})
		return
	}
	if r.URL.Path == "/me" && r.Method == "GET" {
		stores, err := a.stores(r.Context(), userID)
		if err != nil {
			fail(503, "store_unavailable")
			return
		}
		reply(200, map[string]any{"user": map[string]string{"id": userID, "username": name}, "stores": stores, "csrfToken": csrf})
		return
	}
	if r.URL.Path == "/stores" || strings.HasPrefix(r.URL.Path, "/stores/") {
		if r.Method == "POST" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
			fail(403, "csrf_rejected")
			return
		}
		a.storeRequest(w, r, userID, reply, fail)
		return
	}
	fail(404, "not_found")
}
