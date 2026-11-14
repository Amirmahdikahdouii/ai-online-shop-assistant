package instagram

import (
	"context"
	"errors"
	"testing"
)

func TestFakePostContract(t *testing.T) {
	ctx := context.Background()
	fake := Fake{}
	identity, err := fake.Verify(ctx, "demo-page")
	if err != nil {
		t.Fatal(err)
	}
	page, err := fake.ListPosts(ctx, identity, "")
	if err != nil || len(page.Posts) != 6 || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	seen := map[string]bool{}
	for _, p := range page.Posts {
		if p.ID == "" || p.Caption == "" || p.PublishedAt.IsZero() || seen[p.ID] {
			t.Fatal(p)
		}
		seen[p.ID] = true
	}
	last, err := fake.ListPosts(ctx, identity, page.NextCursor)
	if err != nil || last.NextCursor != "" || len(last.Posts) != 2 {
		t.Fatal(last, err)
	}
	for _, p := range last.Posts {
		if seen[p.ID] {
			t.Fatal("duplicate page item")
		}
	}
	if _, err = fake.ListPosts(ctx, identity, "-1"); !errors.Is(err, ErrCursor) {
		t.Fatal(err)
	}
	identity.AccountID = "foreign-account"
	if _, err = fake.ListPosts(ctx, identity, ""); err == nil {
		t.Fatal("foreign account accepted")
	}
}
