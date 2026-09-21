package shop

import (
	"context"
	"errors"
	"strings"

	"github.com/stripe/stripe-go/v86"
)

type stripeClient struct {
	client *stripe.Client
}

// NewStripeClient returns a client, or nil when no API key is configured so the
// caller can leave checkout disabled.
func NewStripeClient(secretKey string) StripeClient {
	if strings.TrimSpace(secretKey) == "" {
		return nil
	}

	return &stripeClient{client: stripe.NewClient(secretKey)}
}

func (c *stripeClient) CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutSession, error) {
	if c.client == nil {
		return nil, ErrStripeNotConfigured
	}

	lineItems := make([]*stripe.CheckoutSessionCreateLineItemParams, 0, len(params.Lines)+1)
	for _, line := range params.Lines {
		lineItems = append(lineItems, lineItem(line.Title, params.Currency, line.UnitPriceCents, line.Quantity))
	}
	if params.ShippingCents > 0 {
		lineItems = append(lineItems, lineItem("Shipping", params.Currency, params.ShippingCents, 1))
	}

	sessionParams := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
		LineItems:  lineItems,
		// Shipping is US-only.
		ShippingAddressCollection: &stripe.CheckoutSessionCreateShippingAddressCollectionParams{
			AllowedCountries: []*string{stripe.String("US")},
		},
	}
	if params.TaxEnabled {
		sessionParams.AutomaticTax = &stripe.CheckoutSessionCreateAutomaticTaxParams{
			Enabled: stripe.Bool(true),
		}
	}
	if params.ExpiresAt > 0 {
		sessionParams.ExpiresAt = stripe.Int64(params.ExpiresAt)
	}

	session, err := c.client.V1CheckoutSessions.Create(ctx, sessionParams)
	if err != nil {
		return nil, err
	}

	return &CheckoutSession{ID: session.ID, URL: session.URL}, nil
}

// ExpireCheckoutSession kills a Checkout Session that must not be paid, for
// example one whose order could not be recorded after its stock was reserved.
func (c *stripeClient) ExpireCheckoutSession(ctx context.Context, sessionID string) error {
	if c.client == nil {
		return ErrStripeNotConfigured
	}

	_, err := c.client.V1CheckoutSessions.Expire(ctx, sessionID, nil)

	return err
}

// RefundPayment refunds the payment behind a Checkout Session. Checkout
// captures payment immediately, so the refund path is normally taken; only an
// uncaptured intent is voided instead.
func (c *stripeClient) RefundPayment(ctx context.Context, paymentIntentID string, idempotencyKey string) error {
	if c.client == nil {
		return ErrStripeNotConfigured
	}
	if strings.TrimSpace(paymentIntentID) == "" {
		return errors.New("shop: missing payment intent id")
	}

	intent, err := c.client.V1PaymentIntents.Retrieve(ctx, paymentIntentID, nil)
	if err != nil {
		return err
	}

	if intent.Status == stripe.PaymentIntentStatusRequiresCapture {
		_, err = c.client.V1PaymentIntents.Cancel(ctx, paymentIntentID, nil)
		return err
	}

	params := &stripe.RefundCreateParams{PaymentIntent: stripe.String(paymentIntentID)}
	params.SetIdempotencyKey(idempotencyKey)

	_, err = c.client.V1Refunds.Create(ctx, params)

	return err
}

func lineItem(name, currency string, unitAmountCents int, quantity int64) *stripe.CheckoutSessionCreateLineItemParams {
	return &stripe.CheckoutSessionCreateLineItemParams{
		Quantity: stripe.Int64(quantity),
		PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(int64(unitAmountCents)),
			ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
				Name: stripe.String(name),
			},
		},
	}
}
