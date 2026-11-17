package identity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"shopassistant/internal/extraction"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
)

type SelectedPost struct {
	instagram.Post
	ProductID       *string                   `json:"productId"`
	Proposal        *extraction.Proposal      `json:"proposal"`
	Reason          string                    `json:"reason"`
	Attempts        int                       `json:"attempts"`
	SelectionID     string                    `json:"selectionId"`
	Status          string                    `json:"status"`
	SelectedAt      time.Time                 `json:"selectedAt"`
	MatchStatus     string                    `json:"matchStatus"`
	MatchEvidence   *extraction.MatchEvidence `json:"matchEvidence"`
	MappingTargets  []PostTarget              `json:"mappingTargets"`
	MappingRevision int                       `json:"mappingRevision"`
}

func (a *Auth) postsRequest(w http.ResponseWriter, r *http.Request, user, store, resource string, reply func(int, map[string]any), fail func(int, string)) {
	ctx := r.Context()
	var member bool
	if err := a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shops.memberships WHERE user_id=$1 AND store_id=$2)`, user, store).Scan(&member); err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if !member {
		fail(404, "not_found")
		return
	}
	if r.Method == "GET" && resource == "post-selections" {
		cursor := r.URL.Query().Get("cursor")
		rows, err := a.Pool.Query(ctx, `SELECT s.id,s.post_id,s.caption,s.published_at,s.status,s.selected_at,s.product_id,s.proposal,coalesce(j.reason,'queued'),coalesce(j.attempts,0),s.match_status,s.match_evidence,s.mapping_targets,s.mapping_revision FROM catalog.post_selections s LEFT JOIN catalog.extraction_jobs j ON j.selection_id=s.id WHERE s.store_id=$1 AND ($2='' OR s.id>$2) ORDER BY s.id LIMIT 21`, store, cursor)
		if err != nil {
			fail(503, "posts_unavailable")
			return
		}
		defer rows.Close()
		items := []SelectedPost{}
		for rows.Next() {
			var p SelectedPost
			var targets json.RawMessage
			if rows.Scan(&p.SelectionID, &p.ID, &p.Caption, &p.PublishedAt, &p.Status, &p.SelectedAt, &p.ProductID, &p.Proposal, &p.Reason, &p.Attempts, &p.MatchStatus, &p.MatchEvidence, &targets, &p.MappingRevision) != nil {
				fail(503, "posts_unavailable")
				return
			}
			if len(targets) > 0 {
				_ = json.Unmarshal(targets, &p.MappingTargets)
			}
			if p.MappingTargets == nil {
				p.MappingTargets = []PostTarget{}
			}
			items = append(items, p)
		}
		if rows.Err() != nil {
			fail(503, "posts_unavailable")
			return
		}
		next := ""
		if len(items) > 20 {
			items = items[:20]
			next = items[19].SelectionID
		}
		reply(200, map[string]any{"selections": items, "nextCursor": next})
		return
	}
	if (r.Method != "GET" || resource != "posts") && (r.Method != "POST" || resource != "post-selections") {
		fail(404, "not_found")
		return
	}
	if a.Instagram == nil {
		fail(503, "posts_unavailable")
		return
	}
	var c Connection
	err := a.Pool.QueryRow(ctx, `SELECT id,provider,account_id,page_id FROM instagram.connections WHERE store_id=$1 AND status='connected'`, store).Scan(&c.ID, &c.Provider, &c.AccountID, &c.PageID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(409, "connection_required")
		return
	}
	if err != nil {
		fail(503, "posts_unavailable")
		return
	}
	identity := instagram.Identity{Provider: c.Provider, AccountID: c.AccountID, PageID: c.PageID}
	if r.Method == "GET" {
		page, err := a.Instagram.ListPosts(ctx, identity, r.URL.Query().Get("cursor"))
		if errors.Is(err, instagram.ErrCursor) {
			fail(422, "invalid_cursor")
			return
		}
		if err != nil {
			fail(503, "posts_unavailable")
			return
		}
		items := []map[string]any{}
		for _, p := range page.Posts {
			var id, status string
			err = a.Pool.QueryRow(ctx, `SELECT id,status FROM catalog.post_selections WHERE store_id=$1 AND connection_id=$2 AND post_id=$3`, store, c.ID, p.ID).Scan(&id, &status)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				fail(503, "posts_unavailable")
				return
			}
			items = append(items, map[string]any{"id": p.ID, "caption": p.Caption, "publishedAt": p.PublishedAt, "selected": id != "", "selectionId": id, "status": status})
		}
		reply(200, map[string]any{"posts": items, "nextCursor": page.NextCursor})
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
		PostIDs []string `json:"postIds"`
		Key     string   `json:"key"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	if len(input.PostIDs) < 1 || len(input.PostIDs) > 20 || len(input.Key) < 8 || len(input.Key) > 100 {
		fail(422, "invalid_selection")
		return
	}
	sort.Strings(input.PostIDs)
	ids := []string{}
	for _, id := range input.PostIDs {
		if len(ids) == 0 || ids[len(ids)-1] != id {
			ids = append(ids, id)
		}
	}
	canonical, _ := json.Marshal(ids)
	hash := digest(string(canonical))
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		fail(503, "posts_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize selection against reconnects and other selection requests.
	var current string
	if err = tx.QueryRow(ctx, `SELECT id FROM instagram.connections WHERE store_id=$1 AND status='connected' FOR UPDATE`, store).Scan(&current); err != nil || current != c.ID {
		fail(409, "connection_changed")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO catalog.post_selection_requests(store_id,user_id,request_key,body_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, store, user, input.Key, hash)
	if err != nil {
		fail(503, "posts_unavailable")
		return
	}
	var savedHash string
	var result map[string]any
	if err = tx.QueryRow(ctx, `SELECT body_hash,result FROM catalog.post_selection_requests WHERE store_id=$1 AND user_id=$2 AND request_key=$3 FOR UPDATE`, store, user, input.Key).Scan(&savedHash, &result); err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if savedHash != hash {
		fail(409, "idempotency_conflict")
		return
	}
	if result != nil {
		reply(200, result)
		return
	}
	// Resolve identifiers only through the authenticated connection's adapter.
	available := map[string]instagram.Post{}
	cursor := ""
	visited := map[string]bool{}
	for {
		if visited[cursor] || len(visited) >= 100 {
			fail(503, "posts_unavailable")
			return
		}
		visited[cursor] = true
		page, e := a.Instagram.ListPosts(ctx, identity, cursor)
		if e != nil {
			fail(503, "posts_unavailable")
			return
		}
		for _, p := range page.Posts {
			available[p.ID] = p
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			fail(503, "posts_unavailable")
			return
		}
		cursor = page.NextCursor
	}
	for _, id := range ids {
		if _, ok := available[id]; !ok {
			fail(422, "invalid_selection")
			return
		}
	}
	items := []SelectedPost{}
	for _, id := range ids {
		p := available[id]
		_, err = tx.Exec(ctx, `INSERT INTO catalog.post_selections(id,store_id,connection_id,post_id,caption,published_at,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(store_id,connection_id,post_id) DO NOTHING`, platform.ID(), store, c.ID, id, p.Caption, p.PublishedAt, user)
		if err != nil {
			fail(503, "posts_unavailable")
			return
		}
		selected := SelectedPost{Post: p}
		if err = tx.QueryRow(ctx, `SELECT id,status,selected_at,caption,published_at FROM catalog.post_selections WHERE store_id=$1 AND connection_id=$2 AND post_id=$3`, store, c.ID, id).Scan(&selected.SelectionID, &selected.Status, &selected.SelectedAt, &selected.Caption, &selected.PublishedAt); err != nil {
			fail(503, "posts_unavailable")
			return
		}
		if err = a.ExtractionQueue.Enqueue(ctx, tx, selected.SelectionID); err != nil {
			fail(503, "posts_unavailable")
			return
		}
		items = append(items, selected)
	}
	result = map[string]any{"selections": items}
	raw, _ := json.Marshal(result)
	if _, err = tx.Exec(ctx, `UPDATE catalog.post_selection_requests SET result=$4 WHERE store_id=$1 AND user_id=$2 AND request_key=$3`, store, user, input.Key, raw); err != nil {
		fail(503, "posts_unavailable")
		return
	}
	if tx.Commit(ctx) != nil {
		fail(503, "posts_unavailable")
		return
	}
	reply(200, result)
}
