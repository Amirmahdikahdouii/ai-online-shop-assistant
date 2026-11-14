package identity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"shopassistant/internal/processing"
)

func (a *Auth) processingRequest(w http.ResponseWriter, r *http.Request, user, store string, reply func(int, map[string]any), fail func(int, string)) {
	var member bool
	if err := a.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM shops.memberships WHERE user_id=$1 AND store_id=$2)`, user, store).Scan(&member); err != nil {
		fail(503, "processing_unavailable")
		return
	}
	if !member {
		fail(404, "not_found")
		return
	}
	if a.Processing == nil {
		fail(503, "processing_unavailable")
		return
	}
	if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/processing") {
		items, err := a.Processing.List(r.Context(), store)
		if err != nil {
			fail(503, "processing_unavailable")
			return
		}
		reply(200, map[string]any{"items": items, "fakeAvailable": a.Instagram != nil, "limit": 100})
		return
	}
	// This authenticated demo gateway accepts fixture names, never arbitrary
	// provider metadata or client-selected speaker roles. Live fails closed.
	if a.Instagram == nil {
		fail(404, "not_found")
		return
	}
	if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/sample-events") {
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
		Scenario string `json:"scenario"`
		Key      string `json:"key"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	validScenario := input.Scenario == "message" || input.Scenario == "failure" || input.Scenario == "old" || input.Scenario == "shop"
	if len(input.Key) < 8 || len(input.Key) > 100 || !validScenario {
		fail(422, "invalid_sample")
		return
	}
	e := processing.Event{ExternalID: input.Scenario + ":" + input.Key, SenderID: "fake-customer", OccurredAt: time.Now().UTC(), Scenario: input.Scenario}
	if input.Scenario == "old" {
		e.OccurredAt = time.Unix(0, 0).UTC()
	}
	if input.Scenario == "shop" {
		// Fixture identity is resolved on the server from the connected page.
		if err = a.Pool.QueryRow(r.Context(), `SELECT page_id FROM instagram.connections WHERE store_id=$1 AND provider='fake'`, store).Scan(&e.SenderID); err != nil {
			fail(409, "connection_required")
			return
		}
	}
	result, err := a.Processing.Accept(r.Context(), store, e)
	if errors.Is(err, processing.ErrConnection) {
		fail(409, "connection_required")
		return
	}
	if err != nil {
		fail(503, "processing_unavailable")
		return
	}
	reply(200, map[string]any{"result": result})
}
