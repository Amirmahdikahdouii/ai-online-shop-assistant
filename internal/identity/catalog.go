package identity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"shopassistant/internal/platform"
)

type Variant struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Code       *string           `json:"code"`
	Attributes map[string]string `json:"attributes"`
	Price      *int64            `json:"price"`
}
type Product struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Revision int       `json:"revision"`
	Variants []Variant `json:"variants"`
}

func catalogText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}
func validateProduct(p *Product) map[string]string {
	fields := map[string]string{}
	p.Name = strings.TrimSpace(p.Name)
	if !catalogText(p.Name, 120) {
		fields["name"] = "نام محصول باید بین ۱ تا ۱۲۰ نویسه باشد."
	}
	if len(p.Variants) < 1 || len(p.Variants) > 20 {
		fields["variants"] = "بین ۱ تا ۲۰ تنوع ثبت کنید."
	}
	codes, ids := map[string]bool{}, map[string]bool{}
	for i := range p.Variants {
		v := &p.Variants[i]
		v.Name = strings.TrimSpace(v.Name)
		if !catalogText(v.Name, 120) {
			fields["variants"] = "نام هر تنوع الزامی است و حداکثر ۱۲۰ نویسه دارد."
		}
		if v.ID != "" && (!catalogText(v.ID, 100) || ids[v.ID]) {
			fields["variants"] = "شناسهٔ تنوع تکراری یا نامعتبر است."
		}
		ids[v.ID] = true
		if v.Code != nil {
			c := strings.ToUpper(strings.TrimSpace(*v.Code))
			if c == "" {
				v.Code = nil
			} else {
				v.Code = &c
				if !catalogText(c, 80) || codes[c] {
					fields["code"] = "کد تنوع باید معتبر و در فروشگاه یکتا باشد."
				}
				codes[c] = true
			}
		}
		if v.Price != nil && (*v.Price < 0 || *v.Price > 9007199254740991) {
			fields["price"] = "قیمت باید مبلغ صحیح و نامنفی به تومان باشد."
		}
		if v.Attributes == nil {
			v.Attributes = map[string]string{}
		}
		if len(v.Attributes) > 20 {
			fields["attributes"] = "حداکثر ۲۰ ویژگی ثبت کنید."
		}
		for k, value := range v.Attributes {
			if !catalogText(k, 80) || !catalogText(value, 200) {
				fields["attributes"] = "نام و مقدار ویژگی را کامل و معتبر وارد کنید."
			}
		}
	}
	return fields
}

func (a *Auth) catalogRequest(w http.ResponseWriter, r *http.Request, user, store, id, action string, reply func(int, map[string]any), fail func(int, string)) {
	ctx := r.Context()
	var member bool
	if err := a.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shops.memberships WHERE user_id=$1 AND store_id=$2)`, user, store).Scan(&member); err != nil {
		fail(503, "catalog_unavailable")
		return
	}
	if !member {
		fail(404, "not_found")
		return
	}
	if r.Method == "GET" {
		if action != "" {
			fail(404, "not_found")
			return
		}
		if id != "" {
			var p Product
			err := a.Pool.QueryRow(ctx, `SELECT id,name,status,revision,variants FROM catalog.products WHERE store_id=$1 AND id=$2`, store, id).Scan(&p.ID, &p.Name, &p.Status, &p.Revision, &p.Variants)
			if errors.Is(err, pgx.ErrNoRows) {
				fail(404, "not_found")
				return
			}
			if err != nil {
				fail(503, "catalog_unavailable")
				return
			}
			rows, err := a.Pool.Query(ctx, `SELECT revision,actor_id,changed_at,snapshot FROM catalog.history WHERE product_id=$1 ORDER BY revision`, id)
			if err != nil {
				fail(503, "catalog_unavailable")
				return
			}
			defer rows.Close()
			history := []map[string]any{}
			for rows.Next() {
				var rev int
				var actor string
				var at any
				var snapshot json.RawMessage
				if rows.Scan(&rev, &actor, &at, &snapshot) != nil {
					fail(503, "catalog_unavailable")
					return
				}
				history = append(history, map[string]any{"revision": rev, "actorId": actor, "changedAt": at, "snapshot": snapshot})
			}
			if rows.Err() != nil {
				fail(503, "catalog_unavailable")
				return
			}
			reply(200, map[string]any{"product": p, "history": history})
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		status := r.URL.Query().Get("status")
		cursor := r.URL.Query().Get("cursor")
		if utf8.RuneCountInString(q) > 120 || len(cursor) > 100 || (status != "" && status != "draft" && status != "approved" && status != "archived") {
			fail(422, "invalid_filter")
			return
		}
		rows, err := a.Pool.Query(ctx, `SELECT id,name,status,revision,variants FROM catalog.products WHERE store_id=$1 AND id>$2 AND ($3='' OR status=$3) AND ($4='' OR strpos(lower(name),lower($4))>0 OR EXISTS(SELECT 1 FROM jsonb_array_elements(variants) v WHERE strpos(lower(v->>'name'),lower($4))>0 OR strpos(lower(coalesce(v->>'code','')),lower($4))>0)) ORDER BY id LIMIT 21`, store, cursor, status, q)
		if err != nil {
			fail(503, "catalog_unavailable")
			return
		}
		defer rows.Close()
		products := []Product{}
		for rows.Next() {
			var p Product
			if rows.Scan(&p.ID, &p.Name, &p.Status, &p.Revision, &p.Variants) != nil {
				fail(503, "catalog_unavailable")
				return
			}
			products = append(products, p)
		}
		if rows.Err() != nil {
			fail(503, "catalog_unavailable")
			return
		}
		next := ""
		if len(products) > 20 {
			products = products[:20]
			next = products[19].ID
		}
		reply(200, map[string]any{"products": products, "nextCursor": next})
		return
	}
	if r.Method != "POST" || (action != "" && action != "approve" && action != "archive") {
		fail(404, "not_found")
		return
	}
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(415, "json_required")
		return
	}
	// Allow the complete supported 20-variant / 20-attribute Unicode form.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2*1024*1024))
	if err != nil {
		fail(413, "body_too_large")
		return
	}
	var input struct {
		Name     string    `json:"name"`
		Variants []Variant `json:"variants"`
		Revision int       `json:"revision"`
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if !validUnicodeJSON(body) || dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF {
		fail(400, "invalid_json")
		return
	}
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		fail(503, "catalog_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	p := Product{ID: id, Name: input.Name, Variants: input.Variants, Status: "draft", Revision: 1}
	var old Product
	if id != "" {
		err = tx.QueryRow(ctx, `SELECT id,name,status,revision,variants FROM catalog.products WHERE store_id=$1 AND id=$2 FOR UPDATE`, store, id).Scan(&old.ID, &old.Name, &old.Status, &old.Revision, &old.Variants)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(404, "not_found")
			return
		}
		if err != nil {
			fail(503, "catalog_unavailable")
			return
		}
		if input.Revision != old.Revision {
			fail(409, "revision_conflict")
			return
		}
		if old.Status == "archived" {
			fail(409, "product_archived")
			return
		}
		p.Revision = old.Revision + 1
		p.Status = old.Status
		if action != "" {
			p.Name = old.Name
			p.Variants = old.Variants
			if action == "approve" {
				p.Status = "approved"
			} else {
				p.Status = "archived"
			}
		}
	} else {
		if action != "" {
			fail(404, "not_found")
			return
		}
		p.ID = platform.ID()
	}
	fields := validateProduct(&p)
	if id != "" && action == "" {
		seen := map[string]bool{}
		for _, v := range p.Variants {
			if v.ID != "" {
				found := false
				for _, o := range old.Variants {
					if v.ID == o.ID {
						found = true
					}
				}
				if !found {
					fields["variants"] = "شناسهٔ تنوع متعلق به این محصول نیست."
				}
				seen[v.ID] = true
			}
		}
		for _, o := range old.Variants {
			if !seen[o.ID] {
				fields["variants"] = "تنوع ثبت‌شده قابل حذف نیست؛ محصول را بایگانی کنید."
			}
		}
	}
	if id == "" {
		for _, v := range p.Variants {
			if v.ID != "" {
				fields["variants"] = "شناسهٔ تنوع جدید را سرور ایجاد می‌کند."
			}
		}
	}
	if len(fields) > 0 {
		reply(422, map[string]any{"code": "validation_failed", "fieldErrors": fields})
		return
	}
	for i := range p.Variants {
		if p.Variants[i].ID == "" {
			p.Variants[i].ID = platform.ID()
		}
	}
	raw, _ := json.Marshal(p.Variants)
	if id == "" {
		_, err = tx.Exec(ctx, `INSERT INTO catalog.products(id,store_id,name,status,revision,variants) VALUES($1,$2,$3,$4,$5,$6)`, p.ID, store, p.Name, p.Status, p.Revision, raw)
	} else {
		_, err = tx.Exec(ctx, `UPDATE catalog.products SET name=$2,status=$3,revision=$4,variants=$5,updated_at=clock_timestamp() WHERE id=$1`, p.ID, p.Name, p.Status, p.Revision, raw)
	}
	if err != nil {
		fail(503, "catalog_unavailable")
		return
	}
	for _, v := range p.Variants {
		if v.Code == nil {
			continue
		}
		var reserved string
		err = tx.QueryRow(ctx, `INSERT INTO catalog.codes(store_id,code,product_id,variant_id) VALUES($1,$2,$3,$4) ON CONFLICT(store_id,code) DO UPDATE SET code=EXCLUDED.code WHERE catalog.codes.product_id=EXCLUDED.product_id AND catalog.codes.variant_id=EXCLUDED.variant_id RETURNING code`, store, *v.Code, p.ID, v.ID).Scan(&reserved)
		if errors.Is(err, pgx.ErrNoRows) {
			reply(409, map[string]any{"code": "code_conflict", "fieldErrors": map[string]string{"code": "این کد در فروشگاه برای محصول یا تنوع دیگری رزرو شده است."}})
			return
		}
		if err != nil {
			fail(503, "catalog_unavailable")
			return
		}
	}
	snapshot, _ := json.Marshal(p)
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.history(product_id,revision,actor_id,snapshot) VALUES($1,$2,$3,$4)`, p.ID, p.Revision, user, snapshot); err != nil {
		fail(503, "catalog_unavailable")
		return
	}
	if tx.Commit(ctx) != nil {
		fail(503, "catalog_unavailable")
		return
	}
	status := 200
	if id == "" {
		status = 201
	}
	reply(status, map[string]any{"product": p})
}
