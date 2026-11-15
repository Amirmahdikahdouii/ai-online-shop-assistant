package identity

import (
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (a *Auth) retryExtraction(w http.ResponseWriter, r *http.Request, user, store, id string, reply func(int, map[string]any), fail func(int, string)) {
	if r.Method != "POST" {
		fail(404, "not_found")
		return
	}
	// Membership is derived from the authenticated session. Completed work cannot
	// be reset, so retries never overwrite a human-edited product.
	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		fail(503, "posts_unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	err = tx.QueryRow(r.Context(), `SELECT s.status FROM catalog.post_selections s JOIN shops.memberships m ON m.store_id=s.store_id WHERE s.id=$1 AND s.store_id=$2 AND m.user_id=$3 FOR UPDATE OF s`, id, store, user).Scan(&status)
	if err == pgx.ErrNoRows {
		fail(404, "not_found")
		return
	}
	if err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if status != "failed" && status != "blocked" {
		fail(409, "extraction_not_retryable")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE catalog.post_selections SET status='pending',proposal=NULL WHERE id=$1 AND product_id IS NULL`, id); err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE catalog.extraction_jobs SET attempts=0,available_at=clock_timestamp(),lease_token=NULL,lease_until=NULL,reason='queued' WHERE selection_id=$1`, id); err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if tx.Commit(r.Context()) != nil {
		fail(503, "posts_unavailable")
		return
	}
	reply(200, map[string]any{"status": "pending"})
}
