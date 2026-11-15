// Package extraction turns caption-only proposals into reviewable catalog drafts.
package extraction

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Input intentionally has no media, URL, arbitrary metadata or catalog access.
type Input struct {
	SourceID    string    `json:"sourceId"`
	Caption     string    `json:"caption"`
	PublishedAt time.Time `json:"publishedAt"`
}
type Source struct {
	ID    string `json:"id"`
	Quote string `json:"quote"`
}

// Unknown has no value/evidence; conflict preserves competing values for review.
type Field struct {
	State   string   `json:"state"`
	Values  []string `json:"values"`
	Sources []Source `json:"sources"`
}
type Proposal struct {
	Outcome    string           `json:"outcome"`
	Name       Field            `json:"name"`
	Code       Field            `json:"code"`
	Price      Field            `json:"price"` // integer tomans, never zero as a substitute for unknown
	Attributes map[string]Field `json:"attributes"`
	Model      string           `json:"model"`
	Policy     string           `json:"policy"`
}

// Metadata comes from server-side adapter configuration, never model output or
// source text, so attempt logs can identify the model without exposing captions.
type Metadata struct {
	Model     string
	Policy    string
	Simulated bool
}
type Extractor interface {
	Extract(context.Context, Input) (Proposal, error)
	Metadata() Metadata
}

var ErrInvalid = errors.New("invalid extraction output")
var ErrRefusal = errors.New("extraction refused")
var ErrBlocked = errors.New("extraction credentials or quota unavailable")

func text(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}
func validate(p Proposal, in Input) error {
	if !text(p.Model, 100) || !text(p.Policy, 100) || len(p.Attributes) > 20 {
		return ErrInvalid
	}
	if p.Outcome != "product" && p.Outcome != "no_product" {
		return ErrInvalid
	}
	fields := []Field{p.Name, p.Code, p.Price}
	for k, f := range p.Attributes {
		if !text(k, 80) {
			return ErrInvalid
		}
		fields = append(fields, f)
	}
	for _, f := range fields {
		switch f.State {
		case "unknown":
			if len(f.Values) != 0 || len(f.Sources) != 0 {
				return ErrInvalid
			}
		case "known":
			if len(f.Values) != 1 || len(f.Sources) < 1 {
				return ErrInvalid
			}
		case "conflict":
			if len(f.Values) < 2 || len(f.Sources) < 2 {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		if len(f.Values) > 20 || len(f.Sources) > 20 {
			return ErrInvalid
		}
		for _, v := range f.Values {
			if !text(v, 200) {
				return ErrInvalid
			}
		}
		for _, s := range f.Sources {
			if s.ID != in.SourceID || s.Quote == "" || len(s.Quote) > 4000 || !strings.Contains(in.Caption, s.Quote) {
				return ErrInvalid
			}
		}
	}
	if p.Name.State == "known" && !text(p.Name.Values[0], 120) {
		return ErrInvalid
	}
	for _, v := range p.Code.Values {
		if !text(v, 80) {
			return ErrInvalid
		}
	}
	for _, v := range p.Price.Values {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 || n > 9007199254740991 {
			return ErrInvalid
		}
	}
	if p.Outcome == "no_product" && (p.Name.State != "unknown" || p.Code.State != "unknown" || p.Price.State != "unknown" || len(p.Attributes) != 0) {
		return ErrInvalid
	}
	return nil
}
func unknown() Field { return Field{State: "unknown", Values: []string{}, Sources: []Source{}} }
func known(in Input, v, q string) Field {
	return Field{State: "known", Values: []string{v}, Sources: []Source{{ID: in.SourceID, Quote: q}}}
}

// ScriptedExtractor recognizes exact demo captions. It does not pretend to infer
// unseen text, contact a model, or inspect images. Fixtures are content keyed.
type ScriptedExtractor struct{}

func (ScriptedExtractor) Metadata() Metadata {
	return Metadata{Model: "scripted-v1", Policy: "caption-v1", Simulated: true}
}

func (ScriptedExtractor) Extract(ctx context.Context, in Input) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	metadata := (ScriptedExtractor{}).Metadata()
	p := Proposal{Outcome: "product", Name: unknown(), Code: unknown(), Price: unknown(), Attributes: map[string]Field{}, Model: metadata.Model, Policy: metadata.Policy}
	switch in.Caption {
	case "مانتو لینن نارون؛ رنگ‌های زیتونی و کرم، سایز ۳۸ تا ۴۴. قیمت هر عدد ۸۹۰٬۰۰۰ تومان.":
		p.Name = known(in, "مانتو لینن نارون", "مانتو لینن نارون")
		p.Price = known(in, "890000", "قیمت هر عدد ۸۹۰٬۰۰۰ تومان")
		p.Attributes["رنگ"] = known(in, "زیتونی و کرم", "رنگ‌های زیتونی و کرم")
		p.Attributes["سایز"] = known(in, "۳۸ تا ۴۴", "سایز ۳۸ تا ۴۴")
	case "شومیز نخی آبی؛ آستین بلند، سایز آزاد. قیمت ۴۵۰٬۰۰۰ تومان.":
		p.Name = known(in, "شومیز نخی آبی", "شومیز نخی آبی")
		p.Price = known(in, "450000", "قیمت ۴۵۰٬۰۰۰ تومان")
		p.Attributes["سایز"] = known(in, "آزاد", "سایز آزاد")
	case "کیف پارچه‌ای طرح برگ؛ رنگ طبیعی، دستهٔ بلند.":
		p.Name = known(in, "کیف پارچه‌ای طرح برگ", "کیف پارچه‌ای طرح برگ")
		p.Attributes["رنگ"] = known(in, "طبیعی", "رنگ طبیعی")
	case "شال نخی ساده؛ رنگ‌های آبی، سبز و سفید.":
		p.Name = known(in, "شال نخی ساده", "شال نخی ساده")
		p.Attributes["رنگ"] = known(in, "آبی، سبز و سفید", "رنگ‌های آبی، سبز و سفید")
	case "پست قدیمی: پالتوی زمستانی خاکستری، سایز ۴۰ و ۴۲.":
		p.Name = known(in, "پالتوی زمستانی خاکستری", "پالتوی زمستانی خاکستری")
		p.Attributes["سایز"] = known(in, "۴۰ و ۴۲", "سایز ۴۰ و ۴۲")
	default:
		p.Outcome = "no_product"
	}
	return p, nil
}
