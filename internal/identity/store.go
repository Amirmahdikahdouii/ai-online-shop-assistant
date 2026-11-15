package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"shopassistant/internal/platform"
)

type Store struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *Auth) stores(ctx context.Context, userID string) ([]Store, error) {
	rows, err := a.Pool.Query(ctx, `SELECT s.id,s.name FROM shops.stores s JOIN shops.memberships m ON m.store_id=s.id WHERE m.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stores := []Store{}
	for rows.Next() {
		var s Store
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		stores = append(stores, s)
	}
	return stores, rows.Err()
}

func (a *Auth) storeRequest(w http.ResponseWriter, r *http.Request, userID string, reply func(int, map[string]any), fail func(int, string)) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/stores/"), "/")
	if len(parts) == 4 && parts[1] == "post-selections" && parts[3] == "mapping" {
		a.postMapping(w, r, userID, parts[0], parts[2], reply, fail)
		return
	}
	if len(parts) == 4 && parts[1] == "post-selections" && parts[3] == "retry" {
		a.retryExtraction(w, r, userID, parts[0], parts[2], reply, fail)
		return
	}
	if len(parts) == 2 && (parts[1] == "posts" || parts[1] == "post-selections") {
		a.postsRequest(w, r, userID, parts[0], parts[1], reply, fail)
		return
	}
	if len(parts) >= 2 && len(parts) <= 4 && parts[1] == "products" {
		id, action := "", ""
		if len(parts) >= 3 {
			id = parts[2]
		}
		if len(parts) == 4 {
			action = parts[3]
		}
		a.catalogRequest(w, r, userID, parts[0], id, action, reply, fail)
		return
	}
	if len(parts) == 2 && (parts[1] == "processing" || parts[1] == "sample-events") {
		a.processingRequest(w, r, userID, parts[0], reply, fail)
		return
	}
	if len(parts) == 3 && parts[1] == "connection" {
		a.connectionRequest(w, r, userID, parts[0], parts[2], reply, fail)
		return
	}
	if r.Method == "GET" {
		stores, err := a.stores(r.Context(), userID)
		if err != nil {
			fail(503, "store_unavailable")
			return
		}
		for _, s := range stores {
			if r.URL.Path == "/stores/"+s.ID {
				var c Connection
				err := a.Pool.QueryRow(r.Context(), `SELECT id,provider,account_id,page_id,status,receiving_since FROM instagram.connections WHERE store_id=$1`, s.ID).Scan(&c.ID, &c.Provider, &c.AccountID, &c.PageID, &c.Status, &c.ReceivingSince)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					fail(503, "connection_unavailable")
					return
				}
				var connection *Connection
				if err == nil {
					connection = &c
				}
				reply(200, map[string]any{"store": s, "connection": connection})
				return
			}
		}
		fail(404, "not_found")
		return
	}
	if r.Method != "POST" || r.URL.Path != "/stores" {
		fail(404, "not_found")
		return
	}
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
		Name string `json:"name"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 80 || strings.ContainsFunc(input.Name, unicode.IsControl) {
		reply(422, map[string]any{"code": "validation_failed", "fieldErrors": map[string]string{"name": "نام فروشگاه باید بین ۱ تا ۸۰ نویسه و بدون نویسهٔ کنترلی باشد."}})
		return
	}
	ctx := r.Context()
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		fail(503, "store_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	s := Store{ID: platform.ID(), Name: input.Name}
	if _, err = tx.Exec(ctx, `INSERT INTO shops.stores(id,name) VALUES($1,$2)`, s.ID, s.Name); err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, userID, s.ID)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		fail(409, "store_already_exists")
		return
	}
	if err != nil {
		fail(503, "store_unavailable")
		return
	}
	if tx.Commit(ctx) != nil {
		fail(503, "store_unavailable")
		return
	}
	reply(201, map[string]any{"store": s})
}
