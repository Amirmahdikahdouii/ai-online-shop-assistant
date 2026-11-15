package identity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type PostTarget struct {
	ProductID string `json:"productId"`
	VariantID string `json:"variantId"`
}
type PostMapping struct {
	Revision   int          `json:"revision"`
	NotProduct bool         `json:"notProduct"`
	Targets    []PostTarget `json:"targets"`
}

func (a *Auth) postMapping(w http.ResponseWriter, r *http.Request, user, store, id string, reply func(int, map[string]any), fail func(int, string)) {
	if r.Method != "GET" && r.Method != "POST" {
		fail(404, "not_found")
		return
	}
	ctx := r.Context()
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		fail(503, "mapping_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var saved PostMapping
	var extractedProduct *string
	// Serialize manager decisions on the selection. Extraction never writes these fields.
	err = tx.QueryRow(ctx, `SELECT s.mapping_revision,s.mapping_not_product,s.mapping_targets,s.product_id FROM catalog.post_selections s JOIN shops.memberships m ON m.store_id=s.store_id WHERE s.id=$1 AND s.store_id=$2 AND m.user_id=$3 FOR UPDATE OF s`, id, store, user).Scan(&saved.Revision, &saved.NotProduct, &saved.Targets, &extractedProduct)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(404, "not_found")
		return
	}
	if err != nil {
		fail(503, "mapping_unavailable")
		return
	}
	if r.Method == "GET" {
		rows, e := tx.Query(ctx, `SELECT revision,actor_id,source,changed_at,snapshot FROM catalog.post_mapping_changes WHERE selection_id=$1 ORDER BY revision`, id)
		if e != nil {
			fail(503, "mapping_unavailable")
			return
		}
		defer rows.Close()
		history := []map[string]any{}
		for rows.Next() {
			var rev int
			var actor, source string
			var at any
			var snapshot PostMapping
			if rows.Scan(&rev, &actor, &source, &at, &snapshot) != nil {
				fail(503, "mapping_unavailable")
				return
			}
			history = append(history, map[string]any{"revision": rev, "actorId": actor, "source": source, "changedAt": at, "snapshot": snapshot})
		}
		if rows.Err() != nil {
			fail(503, "mapping_unavailable")
			return
		}
		reply(200, map[string]any{"mapping": saved, "history": history})
		return
	}
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(415, "json_required")
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
	if e != nil {
		fail(413, "body_too_large")
		return
	}
	var input PostMapping
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	if input.Revision < 0 || len(input.Targets) > 100 || (input.NotProduct && len(input.Targets) > 0) || (!input.NotProduct && len(input.Targets) == 0) {
		fail(422, "invalid_mapping")
		return
	}
	sort.Slice(input.Targets, func(i, j int) bool {
		a, b := input.Targets[i], input.Targets[j]
		if a.ProductID == b.ProductID {
			return a.VariantID < b.VariantID
		}
		return a.ProductID < b.ProductID
	})
	if input.Targets == nil {
		input.Targets = []PostTarget{}
	}
	for i, t := range input.Targets {
		if t.ProductID == "" || (i > 0 && input.Targets[i-1].ProductID == t.ProductID && (input.Targets[i-1].VariantID == "" || input.Targets[i-1].VariantID == t.VariantID)) {
			fail(422, "invalid_mapping")
			return
		}
	}
	// An identical retry is a no-op, even after the caller lost the response.
	current, _ := json.Marshal(saved.Targets)
	targets, _ := json.Marshal(input.Targets)
	if saved.Revision > 0 && saved.NotProduct == input.NotProduct && string(current) == string(targets) && (input.Revision == saved.Revision || input.Revision == saved.Revision-1) {
		reply(200, map[string]any{"mapping": saved})
		return
	}
	if input.Revision != saved.Revision {
		fail(409, "revision_conflict")
		return
	}
	for _, target := range input.Targets {
		var variants []Variant
		var status string
		err = tx.QueryRow(ctx, `SELECT variants,status FROM catalog.products WHERE id=$1 AND store_id=$2 FOR SHARE`, target.ProductID, store).Scan(&variants, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(422, "invalid_target")
			return
		}
		if err != nil {
			fail(503, "mapping_unavailable")
			return
		}
		found := target.VariantID == ""
		for _, v := range variants {
			if v.ID == target.VariantID {
				found = true
			}
		}
		if !found || status == "archived" {
			fail(422, "invalid_target")
			return
		}
	}
	input.Revision = saved.Revision + 1
	snapshot, _ := json.Marshal(input)
	if _, err = tx.Exec(ctx, `UPDATE catalog.post_selections SET mapping_revision=$2,mapping_not_product=$3,mapping_targets=$4 WHERE id=$1`, id, input.Revision, input.NotProduct, targets); err != nil {
		fail(503, "mapping_unavailable")
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.post_mapping_changes(selection_id,revision,actor_id,source,snapshot) VALUES($1,$2,$3,'manager',$4)`, id, input.Revision, user, snapshot); err != nil {
		fail(503, "mapping_unavailable")
		return
	}

	// Retire only the untouched machine-created candidate when the manager
	// explicitly chooses another identity (or no product). Human edits and
	// candidates reused by another post remain available.
	if extractedProduct != nil {
		retained := false
		for _, target := range input.Targets {
			if target.ProductID == *extractedProduct {
				retained = true
			}
		}
		if !retained {
			var archived Product
			err = tx.QueryRow(ctx, `UPDATE catalog.products p SET status='archived',revision=2,updated_at=clock_timestamp()
       WHERE p.id=$1 AND p.store_id=$2 AND p.status='draft' AND p.revision=1
       AND NOT EXISTS (SELECT 1 FROM catalog.post_selections s, jsonb_array_elements(s.mapping_targets) t WHERE s.store_id=$2 AND t->>'productId'=p.id)
       RETURNING id,name,status,revision,variants`, *extractedProduct, store).Scan(&archived.ID, &archived.Name, &archived.Status, &archived.Revision, &archived.Variants)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				fail(503, "mapping_unavailable")
				return
			}
			if err == nil {
				raw, _ := json.Marshal(archived)
				if _, err = tx.Exec(ctx, `INSERT INTO catalog.history(product_id,revision,actor_id,snapshot) VALUES($1,$2,$3,$4)`, archived.ID, archived.Revision, user, raw); err != nil {
					fail(503, "mapping_unavailable")
					return
				}
			}
		}
	}
	if tx.Commit(ctx) != nil {
		fail(503, "mapping_unavailable")
		return
	}
	reply(200, map[string]any{"mapping": input})
}
