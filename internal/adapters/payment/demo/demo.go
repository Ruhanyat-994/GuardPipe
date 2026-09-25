// Package demo is the stand-in payment provider used until a real one
// (Stripe) is wired: it never talks to a network and never sees card data.
// Its "checkout page" is GuardPipe's own /checkout/{id} demo page, which
// completes the purchase through POST /billing/checkout/{id}/confirm.
package demo

import (
	"context"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
)

type Provider struct{}

var _ billing.PaymentProvider = Provider{}

func (Provider) Name() string { return "demo" }

// CreateSession returns the frontend route of the demo checkout page.
func (Provider) CreateSession(_ context.Context, s billing.CheckoutSession) (string, string, error) {
	return "/checkout/" + s.ID.String(), "demo_" + s.ID.String(), nil
}
