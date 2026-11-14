// Package identity owns account registration; credentials never leave this boundary.
package identity

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"shopassistant/internal/platform"
)

// RegisterHandler limits expensive hashing per process, without a new service.
func RegisterHandler(pool *pgxpool.Pool) http.Handler {
	slots := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := platform.ID()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Request-ID", requestID)
		fail := func(status int, code string, fields map[string]string) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"code": code, "fieldErrors": fields, "requestId": requestID})
		}
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "application/json" {
			fail(415, "json_required", nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fail(413, "body_too_large", nil)
			return
		}
		if !validUnicodeJSON(body) {
			fail(400, "invalid_json", nil)
			return
		}
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&input)
		if err == nil {
			var extra any
			err = decoder.Decode(&extra)
			if err == io.EOF {
				err = nil
			} else if err == nil {
				err = errors.New("extra JSON")
			}
		}
		if err != nil {
			var oversized *http.MaxBytesError
			if errors.As(err, &oversized) {
				fail(413, "body_too_large", nil)
			} else {
				fail(400, "invalid_json", nil)
			}
			return
		}
		username := cases.Fold().String(norm.NFKC.String(strings.TrimSpace(input.Username)))
		username = strings.NewReplacer("ي", "ی", "ى", "ی", "ك", "ک").Replace(username)
		fields := map[string]string{}
		length := utf8.RuneCountInString(username)
		valid, substantive := length >= 3 && length <= 32, false
		for _, c := range username {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				substantive = true
			} else if !unicode.IsMark(c) && c != '_' && c != '.' && c != '-' {
				valid = false
			}
		}
		if !valid || !substantive {
			fields["username"] = "نام کاربری باید ۳ تا ۳۲ حرف یا عدد باشد؛ نقطه، خط تیره و زیرخط هم مجاز است."
		}
		if n := utf8.RuneCountInString(input.Password); n < 12 || n > 128 || len(input.Password) > 512 {
			fields["password"] = "رمز عبور باید بین ۱۲ تا ۱۲۸ نویسه باشد."
		}
		if len(fields) > 0 {
			fail(422, "validation_failed", fields)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.Header().Set("Retry-After", "2")
			fail(429, "registration_busy", nil)
			return
		}
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			fail(503, "registration_unavailable", nil)
			return
		}
		hash := argon2.IDKey([]byte(input.Password), salt, 3, 64*1024, 1, 32)
		encoding := base64.RawStdEncoding
		phc := fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=1$%s$%s", encoding.EncodeToString(salt), encoding.EncodeToString(hash))
		id := platform.ID()
		var created time.Time
		err = pool.QueryRow(r.Context(), `INSERT INTO identity.users(id, username, password_hash) VALUES ($1,$2,$3) RETURNING created_at`, id, username, phc).Scan(&created)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_username_key" {
				fail(409, "username_taken", map[string]string{"username": "این نام کاربری قبلاً ثبت شده است؛ نام دیگری انتخاب کنید."})
			} else {
				fail(503, "registration_unavailable", nil)
			}
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": id, "username": username, "createdAt": created.UTC().Format(time.RFC3339Nano)}, "requestId": requestID})
	})
}

// encoding/json replaces invalid UTF-8 and unpaired surrogate escapes. Reject
// those instead, so a credential is never silently changed before hashing.
func validUnicodeJSON(body []byte) bool {
	if !utf8.Valid(body) {
		return false
	}
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) {
			return false
		}
		if body[i] != 'u' {
			continue
		}
		if i+4 >= len(body) {
			return false
		}
		value, err := strconv.ParseUint(string(body[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(body) || body[i+1] != '\\' || body[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(body[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
