package identity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"shopassistant/internal/platform"
)

type Connection struct {
	ID             string    `json:"id"`
	Provider       string    `json:"provider"`
	AccountID      string    `json:"accountId"`
	PageID         string    `json:"pageId"`
	Status         string    `json:"status"`
	ReceivingSince time.Time `json:"receivingSince"`
}

func (a *Auth) connectionRequest(w http.ResponseWriter, r *http.Request, userID, storeID, action string, reply func(int, map[string]any), fail func(int, string)) {
	ctx := r.Context()
	var member bool
	if err := a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shops.memberships WHERE user_id=$1 AND store_id=$2)`, userID, storeID).Scan(&member); err != nil {
		fail(503, "connection_unavailable")
		return
	}
	if !member {
		fail(404, "not_found")
		return
	}
	if action == "status" && r.Method == "GET" {
		var c Connection
		err := a.Pool.QueryRow(ctx, `SELECT id,provider,account_id,page_id,status,receiving_since FROM instagram.connections WHERE store_id=$1`, storeID).Scan(&c.ID, &c.Provider, &c.AccountID, &c.PageID, &c.Status, &c.ReceivingSince)
		if errors.Is(err, pgx.ErrNoRows) {
			reply(200, map[string]any{"connection": nil, "fakeAvailable": a.Instagram != nil})
			return
		}
		if err != nil {
			fail(503, "connection_unavailable")
			return
		}
		reply(200, map[string]any{"connection": c, "fakeAvailable": a.Instagram != nil})
		return
	}
	if r.Method != "POST" || (action != "start" && action != "callback" && action != "disconnect") {
		fail(404, "not_found")
		return
	}
	if a.Instagram == nil {
		fail(503, "provider_not_configured")
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
		State string `json:"state"`
		Code  string `json:"code"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	if action == "start" {
		state := platform.ID() + platform.ID()
		if a.Instagram.Begin(ctx, state) != nil {
			fail(503, "connection_unavailable")
			return
		}
		_, err = a.Pool.Exec(ctx, `INSERT INTO instagram.connection_attempts(state_hash,store_id,user_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '10 minutes')`, digest(state), storeID, userID)
		if err != nil {
			fail(503, "connection_unavailable")
			return
		}
		reply(200, map[string]any{"state": state, "provider": "fake"})
		return
	}
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		fail(503, "connection_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize all lifecycle changes for a store, including callback replays.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM shops.stores WHERE id=$1 FOR UPDATE`, storeID).Scan(&locked); err != nil {
		fail(503, "connection_unavailable")
		return
	}
	if action == "disconnect" {
		_, err = tx.Exec(ctx, `UPDATE instagram.connections SET status='disconnected' WHERE store_id=$1`, storeID)
		// An outstanding callback cannot silently undo a later disconnect.
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM instagram.connection_attempts WHERE store_id=$1`, storeID)
		}
	} else {
		var completed bool
		err = tx.QueryRow(ctx, `SELECT completed FROM instagram.connection_attempts WHERE state_hash=$1 AND store_id=$2 AND user_id=$3 AND expires_at>clock_timestamp() FOR UPDATE`, digest(input.State), storeID, userID).Scan(&completed)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(400, "invalid_state")
			return
		}
		if err != nil {
			fail(503, "connection_unavailable")
			return
		}
		if completed {
			reply(200, map[string]any{"completed": true})
			return
		}
		identity, verifyErr := a.Instagram.Verify(ctx, input.Code)
		if verifyErr != nil {
			fail(400, "invalid_callback")
			return
		}
		var previous Connection
		err = tx.QueryRow(ctx, `SELECT id,provider,account_id,page_id,status,receiving_since FROM instagram.connections WHERE store_id=$1`, storeID).Scan(&previous.ID, &previous.Provider, &previous.AccountID, &previous.PageID, &previous.Status, &previous.ReceivingSince)
		if err == nil {
			if previous.Provider != identity.Provider || previous.PageID != identity.PageID || previous.AccountID != identity.AccountID {
				fail(409, "page_change_not_allowed")
				return
			}
			_, err = tx.Exec(ctx, `UPDATE instagram.connections SET status='connected' WHERE store_id=$1`, storeID)
		} else if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,$3,$4,$5,'connected')`, platform.ID(), storeID, identity.Provider, identity.AccountID, identity.PageID)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(409, "page_already_linked")
			return
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE instagram.connection_attempts SET completed=true WHERE state_hash=$1`, digest(input.State))
		}
	}
	if err != nil || tx.Commit(ctx) != nil {
		fail(503, "connection_unavailable")
		return
	}
	reply(200, map[string]any{"completed": true})
}
