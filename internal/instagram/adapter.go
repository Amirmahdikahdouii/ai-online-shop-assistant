// Package instagram defines the provider seam. Fake never contacts a network or
// accepts client-supplied provider identities. Live integration is separate work.
package instagram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Identity struct {
	Provider  string
	AccountID string
	PageID    string
}

type Adapter interface {
	Begin(context.Context, string) error
	Verify(context.Context, string) (Identity, error)
	ListPosts(context.Context, Identity, string) (PostPage, error)
}

type Post struct {
	ID          string    `json:"id"`
	Caption     string    `json:"caption"`
	PublishedAt time.Time `json:"publishedAt"`
}

type PostPage struct {
	Posts      []Post `json:"posts"`
	NextCursor string `json:"nextCursor"`
}

var ErrCursor = errors.New("invalid post cursor")

// ListPosts is deterministic, page-scoped and entirely offline. Dates deliberately
// predate connection so browsing old posts never implies importing old messages.
func (Fake) ListPosts(_ context.Context, identity Identity, cursor string) (PostPage, error) {
	if identity.Provider != "fake" || (identity.PageID != "demo-page" && identity.PageID != "other-page") || identity.AccountID != "fake-account-"+identity.PageID {
		return PostPage{}, errors.New("unknown fake page")
	}
	start := 0
	if cursor != "" {
		var err error
		start, err = strconv.Atoi(cursor)
		if err != nil || start != 6 {
			return PostPage{}, ErrCursor
		}
	}
	captions := []string{"مانتو لینن نارون؛ رنگ‌های زیتونی و کرم، سایز ۳۸ تا ۴۴. قیمت هر عدد ۸۹۰٬۰۰۰ تومان.", "شومیز نخی آبی؛ آستین بلند، سایز آزاد. قیمت ۴۵۰٬۰۰۰ تومان.", "کیف پارچه‌ای طرح برگ؛ رنگ طبیعی، دستهٔ بلند.", "شال نخی ساده؛ رنگ‌های آبی، سبز و سفید.", "ست تابستانی لینن؛ پیراهن و شلوار، دو تنوع مستقل.", "جزئیات دوخت مانتو نارون و راهنمای اندازه‌گیری.", "پست قدیمی: پالتوی زمستانی خاکستری، سایز ۴۰ و ۴۲.", "پشت صحنهٔ آماده‌سازی مجموعه؛ این پست معرفی محصول ندارد."}
	page := PostPage{Posts: []Post{}}
	end := min(start+6, len(captions))
	for i := start; i < end; i++ {
		page.Posts = append(page.Posts, Post{ID: fmt.Sprintf("%s-post-%02d", identity.PageID, i+1), Caption: captions[i], PublishedAt: time.Date(2022, 6, 20-i, 10, 0, 0, 0, time.UTC)})
	}
	if end < len(captions) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

type Fake struct{}

func (Fake) Begin(context.Context, string) error { return nil }
func (Fake) Verify(_ context.Context, code string) (Identity, error) {
	switch code {
	case "demo-page", "other-page":
		return Identity{Provider: "fake", AccountID: "fake-account-" + code, PageID: code}, nil
	default:
		return Identity{}, errors.New("invalid fake callback")
	}
}
