package identity

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/extraction"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
)

func TestAutoMatching(t *testing.T) {
	url := os.Getenv("REGISTRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = platform.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	userA, userB := platform.ID(), platform.ID()
	storeA, storeB := platform.ID(), platform.ID()
	connA, connB := platform.ID(), platform.ID()
	tokenA, tokenB := platform.ID(), platform.ID()
	csrf := platform.ID()

	for _, s := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO identity.users(id,username,password_hash) VALUES($1,$2,'unused')`, []any{userA, "match_" + strings.ToLower(userA)}},
		{`INSERT INTO identity.users(id,username,password_hash) VALUES($1,$2,'unused')`, []any{userB, "match_" + strings.ToLower(userB)}},
		{`INSERT INTO shops.stores(id,name) VALUES($1,'فروشگاه الف')`, []any{storeA}},
		{`INSERT INTO shops.stores(id,name) VALUES($1,'فروشگاه ب')`, []any{storeB}},
		{`INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, []any{userA, storeA}},
		{`INSERT INTO shops.memberships(user_id,store_id) VALUES($1,$2)`, []any{userB, storeB}},
		{`INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{digest(tokenA), userA, csrf}},
		{`INSERT INTO identity.sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{digest(tokenB), userB, csrf}},
		{`INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,'fake','fake-acct-A','page-A','connected')`, []any{connA, storeA}},
		{`INSERT INTO instagram.connections(id,store_id,provider,account_id,page_id,status) VALUES($1,$2,'fake','fake-acct-B','page-B','connected')`, []any{connB, storeB}},
	} {
		if _, err = pool.Exec(ctx, s.q, s.args...); err != nil {
			t.Fatal(err)
		}
	}

	defer func() {
		for _, s := range []string{storeA, storeB} {
			for _, q := range []string{
				`DELETE FROM catalog.post_mapping_changes WHERE selection_id IN (SELECT id FROM catalog.post_selections WHERE store_id=$1)`,
				`DELETE FROM catalog.extraction_jobs WHERE selection_id IN (SELECT id FROM catalog.post_selections WHERE store_id=$1)`,
				`DELETE FROM catalog.post_selection_requests WHERE store_id=$1`,
				`DELETE FROM catalog.post_selections WHERE store_id=$1`,
				`DELETE FROM catalog.history WHERE product_id IN (SELECT id FROM catalog.products WHERE store_id=$1)`,
				`DELETE FROM catalog.codes WHERE store_id=$1`,
				`DELETE FROM catalog.products WHERE store_id=$1`,
				`DELETE FROM instagram.connections WHERE store_id=$1`,
				`DELETE FROM shops.memberships WHERE store_id=$1`,
				`DELETE FROM shops.stores WHERE id=$1`,
			} {
				_, _ = pool.Exec(ctx, q, s)
			}
		}
		for _, u := range []string{userA, userB} {
			_, _ = pool.Exec(ctx, `DELETE FROM identity.sessions WHERE user_id=$1`, u)
			_, _ = pool.Exec(ctx, `DELETE FROM identity.users WHERE id=$1`, u)
		}
	}()

	authA := NewAuth(pool, "https://shop.example", true)
	authA.Instagram = instagram.Fake{}

	callA := func(method, path, body string, want int) []byte {
		r := httptest.NewRequest(method, "/stores/"+storeA+path, strings.NewReader(body))
		r.Header.Set("Origin", authA.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", "shop_session="+tokenA)
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		authA.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}

	// 1. Create approved catalog product in Store A with explicit code LIN-01
	linVariantID := platform.ID()
	linProductID := platform.ID()
	linVariants, _ := json.Marshal([]map[string]any{{
		"id":         linVariantID,
		"name":       "آبی متوسط",
		"code":       "LIN-01",
		"price":      520000,
		"attributes": map[string]string{"مدل": "خنک و تابستانی"},
	}})
	if _, err = pool.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'approved',1,$4)`, linProductID, storeA, "پیراهن لینن نورا", linVariants); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO catalog.codes(store_id,code,product_id,variant_id) VALUES($1,'LIN-01',$2,$3)`, storeA, linProductID, linVariantID); err != nil {
		t.Fatal(err)
	}

	// 2. Create another approved product in Store A: "شال نخی ساده" (no code)
	shalVariantID := platform.ID()
	shalProductID := platform.ID()
	shalVariants, _ := json.Marshal([]map[string]any{{
		"id":         shalVariantID,
		"name":       "ساده",
		"code":       nil,
		"price":      180000,
		"attributes": map[string]string{"رنگ": "آبی، سبز و سفید"},
	}})
	if _, err = pool.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'approved',1,$4)`, shalProductID, storeA, "شال نخی ساده", shalVariants); err != nil {
		t.Fatal(err)
	}

	// 3. Create approved product in Store A: "مانتو لینن نارون"
	mantoVariantID := platform.ID()
	mantoProductID := platform.ID()
	mantoVariants, _ := json.Marshal([]map[string]any{{
		"id":         mantoVariantID,
		"name":       "زیتونی و کرم",
		"code":       nil,
		"price":      890000,
		"attributes": map[string]string{"سایز": "۳۸ تا ۴۴"},
	}})
	if _, err = pool.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'approved',1,$4)`, mantoProductID, storeA, "مانتو لینن نارون", mantoVariants); err != nil {
		t.Fatal(err)
	}

	// Also create a DRAFT product in Store A: "پالتوی زمستانی خاکستری"
	draftProductID := platform.ID()
	draftVariants, _ := json.Marshal([]map[string]any{{
		"id":         platform.ID(),
		"name":       "خاکستری",
		"code":       "PALTO-01",
		"price":      1200000,
		"attributes": map[string]string{"سایز": "۴۰ و ۴۲"},
	}})
	if _, err = pool.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,'draft',1,$4)`, draftProductID, storeA, "پالتوی زمستانی خاکستری", draftVariants); err != nil {
		t.Fatal(err)
	}

	worker := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool}, Extractor: extraction.ScriptedExtractor{}, Timeout: time.Second}

	// Helper to insert a post selection directly and process it
	insertAndProcess := func(postID, caption string, publishedAt time.Time) string {
		selID := platform.ID()
		_, err := pool.Exec(ctx, `INSERT INTO catalog.post_selections(id,store_id,connection_id,post_id,caption,published_at,actor_id,status) VALUES($1,$2,$3,$4,$5,$6,$7,'pending')`, selID, storeA, connA, postID, caption, publishedAt, userA)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO catalog.extraction_jobs(selection_id) VALUES($1)`, selID)
		if err != nil {
			t.Fatal(err)
		}
		if err = worker.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		return selID
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Scenario 1: Explicit Code Match
	t.Run("ExplicitCodeMatch", func(t *testing.T) {
		selID := insertAndProcess("post-code-01", "پیراهن لینن نورا؛ کد LIN-01، خنک و تابستانی. قیمت ۵۲۰٬۰۰۰ تومان.", now)
		var matchStatus string
		var evidenceRaw []byte
		var targetsRaw []byte
		var rev int
		var draftID *string
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence, mapping_targets, mapping_revision, product_id FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw, &targetsRaw, &rev, &draftID)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "matched" {
			t.Fatalf("expected match_status=matched, got %s", matchStatus)
		}
		if draftID != nil {
			t.Fatalf("explicit code match must not create draft product, got %v", *draftID)
		}
		if rev != 1 {
			t.Fatalf("expected mapping_revision=1, got %d", rev)
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) != 1 || targets[0].ProductID != linProductID || targets[0].VariantID != linVariantID {
			t.Fatalf("unexpected targets: %+v", targets)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "explicit_code" || ev.Policy != "caption-v1" {
			t.Fatalf("unexpected evidence: %+v", ev)
		}
		if !strings.Contains(ev.Summary, "LIN-01") {
			t.Fatalf("summary should mention code: %s", ev.Summary)
		}
	})

	// Scenario 2: Multiple posts for one product (چند پست یک محصول)
	t.Run("MultiPostSingleProduct", func(t *testing.T) {
		selID := insertAndProcess("post-code-02", "شارژ مجدد پیراهن لینن نورا؛ کد LIN-01 دوباره موجود شد. ارسال فوری.", now)
		var matchStatus string
		var targetsRaw []byte
		var draftID *string
		err := pool.QueryRow(ctx, `SELECT match_status, mapping_targets, product_id FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &targetsRaw, &draftID)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "matched" {
			t.Fatalf("expected match_status=matched, got %s", matchStatus)
		}
		if draftID != nil {
			t.Fatalf("second post must not create another draft product: %v", *draftID)
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) != 1 || targets[0].ProductID != linProductID {
			t.Fatalf("second post must target same product, got %+v", targets)
		}
	})

	// Scenario 3: Unknown Code -> Unresolved (کد ناشناخته)
	t.Run("UnknownCodeUnresolved", func(t *testing.T) {
		selID := insertAndProcess("post-code-unk", "کت جین پاییزه؛ کد UNK-99، تک‌رنگ آبی تیره. قیمت ۹۸۰٬۰۰۰ تومان.", now)
		var matchStatus string
		var evidenceRaw []byte
		var rev int
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence, mapping_revision FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw, &rev)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "unresolved" {
			t.Fatalf("expected match_status=unresolved for unknown code, got %s", matchStatus)
		}
		if rev != 0 {
			t.Fatalf("unresolved match must not increment mapping_revision, got %d", rev)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "unknown_code" {
			t.Fatalf("expected reason=unknown_code, got %s", ev.Reason)
		}
	})

	// Scenario 4: Clear Caption Match without Code (تطبیق روشن بدون کد)
	t.Run("CaptionContextMatchWithoutCode", func(t *testing.T) {
		selID := insertAndProcess("post-shal-01", "شال نخی ساده؛ رنگ‌های آبی، سبز و سفید.", now)
		var matchStatus string
		var evidenceRaw []byte
		var targetsRaw []byte
		var rev int
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence, mapping_targets, mapping_revision FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw, &targetsRaw, &rev)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "matched" {
			t.Fatalf("expected match_status=matched for clear caption match, got %s", matchStatus)
		}
		if rev != 1 {
			t.Fatalf("expected mapping_revision=1, got %d", rev)
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) != 1 || targets[0].ProductID != shalProductID {
			t.Fatalf("expected target to be shalProductID, got %+v", targets)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "caption_context" {
			t.Fatalf("expected reason=caption_context, got %s", ev.Reason)
		}
	})

	// Scenario 5: Similar product must NOT be merged (محصول مشابه - شباهت تنها)
	t.Run("SimilarProductNotMerged", func(t *testing.T) {
		selID := insertAndProcess("post-similar-01", "پیراهن لینن نورا آستین کوتاه؛ رنگ خردلی، سایز بزرگ.", now)
		var matchStatus string
		var evidenceRaw []byte
		var targetsRaw []byte
		var rev int
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence, mapping_targets, mapping_revision FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw, &targetsRaw, &rev)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "unresolved" {
			t.Fatalf("expected match_status=unresolved for similar product, got %s", matchStatus)
		}
		if rev != 0 {
			t.Fatalf("similar product must not auto-map, got rev=%d", rev)
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) != 0 {
			t.Fatalf("must not merge into existing product: %+v", targets)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "similarity_only" {
			t.Fatalf("expected reason=similarity_only, got %s", ev.Reason)
		}
	})

	// Scenario 6: Multi-product post (پست چندمحصولی)
	t.Run("MultiProductPostUnresolved", func(t *testing.T) {
		selID := insertAndProcess("post-multi-01", "ست مانتو لینن نارون و شال نخی ساده؛ ترکیب رنگ زیتونی و سفید.", now)
		var matchStatus string
		var evidenceRaw []byte
		var rev int
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence, mapping_revision FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw, &rev)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "unresolved" {
			t.Fatalf("multi-product post must be unresolved, got %s", matchStatus)
		}
		if rev != 0 {
			t.Fatalf("multi-product post must not auto-save mapping, rev=%d", rev)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "multiple_candidates" {
			t.Fatalf("expected reason=multiple_candidates, got %s", ev.Reason)
		}
		if len(ev.Candidates) < 2 {
			t.Fatalf("expected at least 2 candidates in evidence, got %+v", ev.Candidates)
		}
	})

	// Scenario 7: Conflicting attributes (ویژگی متعارض)
	t.Run("ConflictingAttributesUnresolved", func(t *testing.T) {
		selID := insertAndProcess("post-conflict-01", "شومیز حریر مجلسی؛ رنگ مشکی در متن و قرمز در استوری، سایز ۳۸.", now)
		var matchStatus string
		var evidenceRaw []byte
		err := pool.QueryRow(ctx, `SELECT match_status, match_evidence FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &evidenceRaw)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus != "unresolved" {
			t.Fatalf("conflicting attributes must be unresolved, got %s", matchStatus)
		}
		var ev extraction.MatchEvidence
		json.Unmarshal(evidenceRaw, &ev)
		if ev.Reason != "conflicting_attributes" {
			t.Fatalf("expected reason=conflicting_attributes, got %s", ev.Reason)
		}
	})

	// Scenario 8: Existing manager mapping has top priority (ارتباط معتبر)
	t.Run("ValidMappingPrecedence", func(t *testing.T) {
		selID := platform.ID()
		// Pre-save manager mapping
		initialTargets, _ := json.Marshal([]PostTarget{{ProductID: mantoProductID, VariantID: mantoVariantID}})
		_, err := pool.Exec(ctx, `INSERT INTO catalog.post_selections(id,store_id,connection_id,post_id,caption,published_at,actor_id,status,mapping_revision,mapping_targets) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',1,$8)`, selID, storeA, connA, "post-premapped-01", "پیراهن لینن نورا؛ کد LIN-01، خنک و تابستانی. قیمت ۵۲۰٬۰۰۰ تومان.", now, userA, initialTargets)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO catalog.extraction_jobs(selection_id) VALUES($1)`, selID)
		if err != nil {
			t.Fatal(err)
		}
		if err = worker.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		var rev int
		var targetsRaw []byte
		err = pool.QueryRow(ctx, `SELECT mapping_revision, mapping_targets FROM catalog.post_selections WHERE id=$1`, selID).Scan(&rev, &targetsRaw)
		if err != nil {
			t.Fatal(err)
		}
		if rev != 1 {
			t.Fatalf("existing manager revision must be preserved, got %d", rev)
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) != 1 || targets[0].ProductID != mantoProductID {
			t.Fatalf("existing manager target must be preserved: %+v", targets)
		}
	})

	// Scenario 9: Draft product in catalog is NEVER auto-matched as approved
	t.Run("DraftProductNeverAutoMatched", func(t *testing.T) {
		selID := insertAndProcess("post-draft-01", "پست قدیمی: پالتوی زمستانی خاکستری، سایز ۴۰ و ۴۲.", now)
		var matchStatus string
		var rev int
		err := pool.QueryRow(ctx, `SELECT match_status, mapping_revision FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &rev)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus == "matched" {
			t.Fatalf("draft product in catalog must never be auto-matched")
		}
	})

	// Scenario 10: Store isolation (نامزدها فقط از همان فروشگاه)
	t.Run("StoreIsolation", func(t *testing.T) {
		selID := platform.ID()
		// In Store B, post caption mentions LIN-01, but Store B has no products with LIN-01
		_, err := pool.Exec(ctx, `INSERT INTO catalog.post_selections(id,store_id,connection_id,post_id,caption,published_at,actor_id,status) VALUES($1,$2,$3,$4,$5,$6,$7,'pending')`, selID, storeB, connB, "post-store-b-01", "پیراهن لینن نورا؛ کد LIN-01، خنک و تابستانی. قیمت ۵۲۰٬۰۰۰ تومان.", now, userB)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO catalog.extraction_jobs(selection_id) VALUES($1)`, selID)
		if err != nil {
			t.Fatal(err)
		}
		if err = worker.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		var matchStatus string
		var targetsRaw []byte
		err = pool.QueryRow(ctx, `SELECT match_status, mapping_targets FROM catalog.post_selections WHERE id=$1`, selID).Scan(&matchStatus, &targetsRaw)
		if err != nil {
			t.Fatal(err)
		}
		if matchStatus == "matched" {
			t.Fatalf("must not match product from another store!")
		}
		var targets []PostTarget
		json.Unmarshal(targetsRaw, &targets)
		if len(targets) > 0 {
			t.Fatalf("targets must be empty for store B, got %+v", targets)
		}
	})

	// API Seam: SelectedPost includes matchStatus, matchEvidence, mappingTargets
	t.Run("APIResponseIncludesMatchData", func(t *testing.T) {
		res := callA("GET", "/post-selections", "", 200)
		var data struct {
			Selections []SelectedPost `json:"selections"`
		}
		if err := json.Unmarshal(res, &data); err != nil {
			t.Fatal(err)
		}
		foundMatched := false
		foundUnresolved := false
		for _, s := range data.Selections {
			if s.MatchStatus == "matched" {
				foundMatched = true
				if s.MatchEvidence == nil || s.MatchEvidence.Reason == "" || s.MatchEvidence.Summary == "" {
					t.Fatalf("matched post missing evidence summary: %+v", s)
				}
			}
			if s.MatchStatus == "unresolved" {
				foundUnresolved = true
				if s.MatchEvidence == nil || s.MatchEvidence.Reason == "" || s.MatchEvidence.Summary == "" {
					t.Fatalf("unresolved post missing evidence summary: %+v", s)
				}
			}
		}
		if !foundMatched || !foundUnresolved {
			t.Fatalf("expected both matched and unresolved in selections: matched=%v, unresolved=%v", foundMatched, foundUnresolved)
		}
	})
}
