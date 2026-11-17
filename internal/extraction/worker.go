package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/platform"
)

type MatchTarget struct {
	ProductID string `json:"productId"`
	VariantID string `json:"variantId"`
}

type catalogVariant struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Code       *string           `json:"code"`
	Price      *int64            `json:"price"`
	Attributes map[string]string `json:"attributes"`
}

type catalogProduct struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Status   string           `json:"status"`
	Revision int              `json:"revision"`
	Variants []catalogVariant `json:"variants"`
}

func matchPost(ctx context.Context, tx pgx.Tx, store string, caption string, p Proposal) (string, *MatchEvidence, []MatchTarget, error) {
	if p.Outcome != "product" {
		return "none", nil, nil, nil
	}

	hasConflict := p.Name.State == "conflict" || p.Code.State == "conflict"
	var conflictQuotes []string
	for _, f := range p.Attributes {
		if f.State == "conflict" {
			hasConflict = true
			for _, s := range f.Sources {
				conflictQuotes = append(conflictQuotes, s.Quote)
			}
		}
	}
	if hasConflict {
		for _, s := range p.Name.Sources {
			conflictQuotes = append(conflictQuotes, s.Quote)
		}
		for _, s := range p.Code.Sources {
			conflictQuotes = append(conflictQuotes, s.Quote)
		}
		return "unresolved", &MatchEvidence{
			Reason:  "conflicting_attributes",
			Summary: "ویژگی‌های متعارض در استخراج محصول یافت شد.",
			Policy:  p.Policy,
			Model:   p.Model,
			Quotes:  conflictQuotes,
		}, nil, nil
	}

	// Priority 2: Explicit Code
	if p.Code.State == "known" && len(p.Code.Values) > 0 && strings.TrimSpace(p.Code.Values[0]) != "" {
		code := strings.ToUpper(strings.TrimSpace(p.Code.Values[0]))
		quotes := []string{}
		for _, s := range p.Code.Sources {
			quotes = append(quotes, s.Quote)
		}

		rows, err := tx.Query(ctx, `SELECT id, name, status, revision, variants FROM catalog.products WHERE store_id=$1 AND status='approved' AND EXISTS (SELECT 1 FROM jsonb_array_elements(variants) v WHERE upper(coalesce(v->>'code','')) = $2)`, store, code)
		if err != nil {
			return "", nil, nil, err
		}
		defer rows.Close()

		var matchedProducts []catalogProduct
		var matchedVariants []catalogVariant
		for rows.Next() {
			var prod catalogProduct
			if err := rows.Scan(&prod.ID, &prod.Name, &prod.Status, &prod.Revision, &prod.Variants); err != nil {
				return "", nil, nil, err
			}
			for _, v := range prod.Variants {
				if v.Code != nil && strings.ToUpper(strings.TrimSpace(*v.Code)) == code {
					matchedProducts = append(matchedProducts, prod)
					matchedVariants = append(matchedVariants, v)
				}
			}
		}
		if rows.Err() != nil {
			return "", nil, nil, rows.Err()
		}

		if len(matchedProducts) == 0 {
			return "unresolved", &MatchEvidence{
				Reason:     "unknown_code",
				Summary:    fmt.Sprintf("کد «%s» در کاتالوگ فروشگاه یافت نشد.", code),
				Policy:     p.Policy,
				Model:      p.Model,
				Quotes:     quotes,
				Candidates: []MatchCandidate{},
			}, nil, nil
		}

		if len(matchedProducts) > 1 {
			var candidates []MatchCandidate
			for i, prod := range matchedProducts {
				candidates = append(candidates, MatchCandidate{
					ProductID:   prod.ID,
					ProductName: prod.Name,
					VariantID:   matchedVariants[i].ID,
					VariantName: matchedVariants[i].Name,
					Code:        code,
				})
			}
			return "unresolved", &MatchEvidence{
				Reason:     "multiple_candidates",
				Summary:    fmt.Sprintf("چند محصول با کد «%s» در کاتالوگ یافت شد.", code),
				Policy:     p.Policy,
				Model:      p.Model,
				Quotes:     quotes,
				Candidates: candidates,
			}, nil, nil
		}

		prod := matchedProducts[0]
		variant := matchedVariants[0]

		contradiction := false
		for k, f := range p.Attributes {
			if f.State == "known" && len(f.Values) > 0 {
				val := f.Values[0]
				if varVal, ok := variant.Attributes[k]; ok {
					if strings.TrimSpace(varVal) != strings.TrimSpace(val) {
						contradiction = true
						break
					}
				}
			}
		}
		if contradiction {
			return "unresolved", &MatchEvidence{
				Reason:  "conflicting_attributes",
				Summary: "کد صریح منطبق است اما مشخصات محصول با کاتالوگ مغایرت دارد.",
				Policy:  p.Policy,
				Model:   p.Model,
				Quotes:  quotes,
				Candidates: []MatchCandidate{{
					ProductID:   prod.ID,
					ProductName: prod.Name,
					VariantID:   variant.ID,
					VariantName: variant.Name,
					Code:        code,
				}},
			}, nil, nil
		}

		return "matched", &MatchEvidence{
			Reason:  "explicit_code",
			Summary: fmt.Sprintf("تطبیق خودکار با کد صریح «%s»", code),
			Policy:  p.Policy,
			Model:   p.Model,
			Quotes:  quotes,
			Candidates: []MatchCandidate{{
				ProductID:   prod.ID,
				ProductName: prod.Name,
				VariantID:   variant.ID,
				VariantName: variant.Name,
				Code:        code,
			}},
		}, []MatchTarget{{ProductID: prod.ID, VariantID: variant.ID}}, nil
	}

	// Priority 3: Caption / Context
	rows, err := tx.Query(ctx, `SELECT id, name, status, revision, variants FROM catalog.products WHERE store_id=$1 AND status='approved' ORDER BY id`, store)
	if err != nil {
		return "", nil, nil, err
	}
	defer rows.Close()

	var allApproved []catalogProduct
	for rows.Next() {
		var prod catalogProduct
		if err := rows.Scan(&prod.ID, &prod.Name, &prod.Status, &prod.Revision, &prod.Variants); err != nil {
			return "", nil, nil, err
		}
		allApproved = append(allApproved, prod)
	}
	if rows.Err() != nil {
		return "", nil, nil, rows.Err()
	}

	extractedName := ""
	nameQuotes := []string{}
	if p.Name.State == "known" && len(p.Name.Values) > 0 {
		extractedName = strings.TrimSpace(p.Name.Values[0])
		for _, s := range p.Name.Sources {
			nameQuotes = append(nameQuotes, s.Quote)
		}
	}

	var mentionedInCaption []catalogProduct
	for _, prod := range allApproved {
		trimmedName := strings.TrimSpace(prod.Name)
		if trimmedName != "" && strings.Contains(caption, trimmedName) {
			mentionedInCaption = append(mentionedInCaption, prod)
		}
	}
	if len(mentionedInCaption) > 1 {
		var candidates []MatchCandidate
		for _, prod := range mentionedInCaption {
			candidates = append(candidates, MatchCandidate{
				ProductID:   prod.ID,
				ProductName: prod.Name,
			})
		}
		return "unresolved", &MatchEvidence{
			Reason:     "multiple_candidates",
			Summary:    "پست چندمحصولی؛ چند محصول در کاتالوگ شناسایی شدند.",
			Policy:     p.Policy,
			Model:      p.Model,
			Quotes:     nameQuotes,
			Candidates: candidates,
		}, nil, nil
	}

	var exactMatches []catalogProduct
	for _, prod := range allApproved {
		if strings.TrimSpace(prod.Name) == extractedName {
			exactMatches = append(exactMatches, prod)
		}
	}
	if len(exactMatches) == 1 {
		prod := exactMatches[0]
		targetVarID := ""
		if len(prod.Variants) == 1 {
			targetVarID = prod.Variants[0].ID
		} else {
			for _, v := range prod.Variants {
				varMatch := true
				for k, f := range p.Attributes {
					if f.State == "known" && len(f.Values) > 0 {
						if vVal, ok := v.Attributes[k]; !ok || strings.TrimSpace(vVal) != strings.TrimSpace(f.Values[0]) {
							varMatch = false
							break
						}
					}
				}
				if varMatch {
					targetVarID = v.ID
					break
				}
			}
		}

		return "matched", &MatchEvidence{
			Reason:  "caption_context",
			Summary: fmt.Sprintf("تطبیق خودکار از روی کپشن و مشخصات با محصول «%s»", prod.Name),
			Policy:  p.Policy,
			Model:   p.Model,
			Quotes:  nameQuotes,
			Candidates: []MatchCandidate{{
				ProductID:   prod.ID,
				ProductName: prod.Name,
				VariantID:   targetVarID,
			}},
		}, []MatchTarget{{ProductID: prod.ID, VariantID: targetVarID}}, nil
	}

	var similarMatches []catalogProduct
	for _, prod := range allApproved {
		pName := strings.TrimSpace(prod.Name)
		if extractedName != "" && pName != "" && extractedName != pName {
			if strings.Contains(extractedName, pName) || strings.Contains(pName, extractedName) {
				similarMatches = append(similarMatches, prod)
			}
		}
	}
	if len(similarMatches) > 0 {
		var candidates []MatchCandidate
		for _, prod := range similarMatches {
			candidates = append(candidates, MatchCandidate{
				ProductID:   prod.ID,
				ProductName: prod.Name,
			})
		}
		return "unresolved", &MatchEvidence{
			Reason:     "similarity_only",
			Summary:    fmt.Sprintf("شباهت ظاهری با محصول «%s» بدون تطبیق قطعی؛ برای جلوگیری از ادغام اشتباه نیازمند بررسی مدیر است.", similarMatches[0].Name),
			Policy:     p.Policy,
			Model:      p.Model,
			Quotes:     nameQuotes,
			Candidates: candidates,
		}, nil, nil
	}

	return "none", nil, nil, nil
}

var ErrLease = errors.New("extraction lease no longer owned")

type Task struct {
	Input
	Token    string
	Attempts int
}
type Queue interface {
	Enqueue(context.Context, pgx.Tx, string) error
	Claim(context.Context) (Task, error)
	Complete(context.Context, Task, Proposal) error
	Fail(context.Context, Task, string, bool) error
}
type PostgresQueue struct {
	Pool        *pgxpool.Pool
	Lease       time.Duration
	MaxAttempts int
}

func (q PostgresQueue) Enqueue(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO catalog.extraction_jobs(selection_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
	return err
}
func (q PostgresQueue) Claim(ctx context.Context) (Task, error) {
	lease := q.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	t := Task{Token: platform.ID()}
	err := q.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT j.selection_id FROM catalog.extraction_jobs j JOIN catalog.post_selections s ON s.id=j.selection_id
 WHERE (s.status='pending' AND j.available_at<=clock_timestamp()) OR (s.status='processing' AND j.lease_until<=clock_timestamp())
 ORDER BY j.available_at,j.selection_id FOR UPDATE OF j,s SKIP LOCKED LIMIT 1
 ), claimed AS (UPDATE catalog.extraction_jobs j SET attempts=attempts+1,lease_token=$1,lease_until=clock_timestamp()+$2*interval '1 second',reason='working'
 FROM candidate c WHERE j.selection_id=c.selection_id RETURNING j.selection_id,j.attempts)
 UPDATE catalog.post_selections s SET status='processing' FROM claimed c WHERE s.id=c.selection_id RETURNING s.id,s.caption,s.published_at,c.attempts`, t.Token, lease.Seconds()).Scan(&t.SourceID, &t.Caption, &t.PublishedAt, &t.Attempts)
	return t, err
}
func (q PostgresQueue) Complete(ctx context.Context, t Task, p Proposal) error {
	if err := validate(p, t.Input); err != nil {
		return err
	}
	tx, err := q.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var store, actor string
	var mappingRevision int
	err = tx.QueryRow(ctx, `SELECT s.store_id,s.actor_id,s.mapping_revision FROM catalog.extraction_jobs j JOIN catalog.post_selections s ON s.id=j.selection_id WHERE j.selection_id=$1 AND j.lease_token=$2 AND j.lease_until>clock_timestamp() AND s.status='processing' FOR UPDATE OF j,s`, t.SourceID, t.Token).Scan(&store, &actor, &mappingRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLease
	}
	if err != nil {
		return err
	}
	var productID *string
	status := "blocked"
	matchStatus := "none"
	var matchEvidenceRaw []byte

	if mappingRevision > 0 {
		status = "completed"
		matchStatus = "matched"
		evidence := &MatchEvidence{
			Reason:  "manager_mapping",
			Summary: "ارتباط از تصمیم قبلی مدیر حفظ شد.",
			Policy:  p.Policy,
			Model:   p.Model,
		}
		matchEvidenceRaw, _ = json.Marshal(evidence)
	} else if p.Outcome == "product" {
		mStatus, evidence, targets, err := matchPost(ctx, tx, store, t.Caption, p)
		if err != nil {
			return err
		}
		matchStatus = mStatus
		if evidence != nil {
			matchEvidenceRaw, _ = json.Marshal(evidence)
		}

		if matchStatus == "matched" && len(targets) > 0 {
			status = "completed"
			targetsJSON, _ := json.Marshal(targets)
			rev := 1
			if _, err = tx.Exec(ctx, `UPDATE catalog.post_selections SET mapping_revision=$2,mapping_not_product=false,mapping_targets=$3 WHERE id=$1`, t.SourceID, rev, targetsJSON); err != nil {
				return err
			}
			snapshot, _ := json.Marshal(map[string]any{
				"revision":   rev,
				"notProduct": false,
				"targets":    targets,
			})
			if _, err = tx.Exec(ctx, `INSERT INTO catalog.post_mapping_changes(selection_id,revision,actor_id,source,snapshot) VALUES($1,$2,$3,'system',$4)`, t.SourceID, rev, actor, snapshot); err != nil {
				return err
			}
		} else {
			status = "completed"
			id := platform.ID()
			productID = &id
			value := func(f Field) string {
				if f.State == "known" {
					return f.Values[0]
				}
				return ""
			}
			name := value(p.Name)
			var code *string
			var price *int64
			if p.Price.State == "known" {
				n, _ := strconv.ParseInt(p.Price.Values[0], 10, 64)
				price = &n
			}
			attrs := map[string]string{}
			for k, f := range p.Attributes {
				if f.State == "known" {
					attrs[k] = value(f)
				}
			}
			variant := map[string]any{"id": platform.ID(), "name": name, "code": code, "price": price, "attributes": attrs}
			variants, _ := json.Marshal([]any{variant})
			if _, err = tx.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'draft',1,$4)`, id, store, name, variants); err != nil {
				return err
			}
			snapshot, _ := json.Marshal(map[string]any{"id": id, "name": name, "status": "draft", "revision": 1, "variants": []any{variant}})
			if _, err = tx.Exec(ctx, `INSERT INTO catalog.history(product_id,revision,actor_id,snapshot) VALUES($1,1,$2,$3)`, id, actor, snapshot); err != nil {
				return err
			}
		}
	}
	reason := p.Outcome
	if mappingRevision > 0 {
		status, reason = "completed", "manager_mapping"
	}
	raw, _ := json.Marshal(p)
	if _, err = tx.Exec(ctx, `UPDATE catalog.post_selections SET status=$2,product_id=$3,proposal=$4,match_status=$5,match_evidence=$6 WHERE id=$1`, t.SourceID, status, productID, raw, matchStatus, matchEvidenceRaw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE catalog.extraction_jobs SET lease_token=NULL,lease_until=NULL,reason=$2 WHERE selection_id=$1`, t.SourceID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (q PostgresQueue) Fail(ctx context.Context, t Task, reason string, retry bool) error {
	max := q.MaxAttempts
	if max <= 0 {
		max = 3
	}
	status := "failed"
	if reason == "unavailable" {
		status = "blocked"
	} else if retry && t.Attempts < max {
		status = "pending"
	}
	delay := time.Duration(1<<min(t.Attempts, 6))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	tag, err := q.Pool.Exec(ctx, `WITH failed AS (UPDATE catalog.extraction_jobs SET lease_token=NULL,lease_until=NULL,reason=$3,available_at=clock_timestamp()+$4*interval '1 second' WHERE selection_id=$1 AND lease_token=$2 AND lease_until>clock_timestamp() RETURNING selection_id) UPDATE catalog.post_selections s SET status=$5 FROM failed f WHERE s.id=f.selection_id AND s.status='processing'`, t.SourceID, t.Token, reason, delay.Seconds(), status)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrLease
	}
	return err
}

type Worker struct {
	Queue     Queue
	Extractor Extractor
	Timeout   time.Duration
}

func (w Worker) ProcessOne(ctx context.Context) error {
	t, err := w.Queue.Claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	started := time.Now()
	metadata := w.Extractor.Metadata()
	outcome := "storage_error"
	defer func() {
		var cost any = "unknown"
		if metadata.Simulated {
			cost = 0.0
		}
		slog.Info("caption extraction attempt", "duration_ms", time.Since(started).Milliseconds(), "outcome", outcome, "model", metadata.Model, "policy", metadata.Policy, "simulated", metadata.Simulated, "cost_usd", cost)
	}()
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	var p Proposal
	if utf8.RuneCountInString(t.Caption) > 12000 || t.PublishedAt.IsZero() {
		err = ErrInvalid
	} else {
		p, err = w.Extractor.Extract(attempt, t.Input)
	}
	if err == nil {
		err = attempt.Err()
	}
	cancel()
	if err == nil {
		err = validate(p, t.Input)
	}
	if err == nil {
		err = w.Queue.Complete(ctx, t, p)
		if err == nil {
			outcome = p.Outcome
		}
		return err
	}
	reason, retry := "provider_error", true
	switch {
	case errors.Is(err, ErrInvalid):
		reason, retry = "invalid_output", false
	case errors.Is(err, ErrRefusal):
		reason, retry = "refused", false
	case errors.Is(err, ErrBlocked):
		reason, retry = "unavailable", false
	case errors.Is(err, context.DeadlineExceeded):
		reason = "timeout"
	}
	outcome = reason
	// Storage settlement uses the parent context, independently of model timeout.
	return w.Queue.Fail(ctx, t, reason, retry)
}
