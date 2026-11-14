// Package instagram defines the provider seam. Fake never contacts a network or
// accepts client-supplied provider identities. Live integration is separate work.
package instagram

import (
	"context"
	"errors"
)

type Identity struct {
	Provider  string
	AccountID string
	PageID    string
}

type Adapter interface {
	Begin(context.Context, string) error
	Verify(context.Context, string) (Identity, error)
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
