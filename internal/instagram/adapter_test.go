package instagram

import (
	"context"
	"testing"
)

func TestFakeUsesStableProviderIdentities(t *testing.T) {
	ctx := context.Background()
	for _, code := range []string{"demo-page", "other-page"} {
		first, err := (Fake{}).Verify(ctx, code)
		if err != nil {
			t.Fatal(err)
		}
		second, err := (Fake{}).Verify(ctx, code)
		if err != nil || first != second || first.Provider != "fake" || first.PageID != code || first.AccountID == "" {
			t.Fatal(first, second, err)
		}
	}
	for _, code := range []string{"", "arbitrary-page", "demo-page "} {
		if _, err := (Fake{}).Verify(ctx, code); err == nil {
			t.Fatal("unverified identity accepted", code)
		}
	}
}
