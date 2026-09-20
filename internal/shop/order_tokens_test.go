package shop

import "testing"

func TestOrderViewTokenRoundTrip(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_view", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	raw, hash, err := NewOrderViewToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || hash == "" {
		t.Fatal("token and hash must both be non-empty")
	}
	if hash != HashOrderViewToken(raw) {
		t.Fatal("stored hash does not match the raw token")
	}

	if _, err := model.MarkOrderPaid("cs_view", PaidDetails{Currency: "usd", ViewTokenHash: hash}); err != nil {
		t.Fatal(err)
	}

	order, err := model.GetOrderByViewToken(hash)
	if err != nil {
		t.Fatal(err)
	}
	if order.StripeSessionID != "cs_view" {
		t.Fatalf("session = %q, want cs_view", order.StripeSessionID)
	}

	if _, err := model.GetOrderByViewToken(HashOrderViewToken("wrong")); err == nil {
		t.Fatal("expected an unknown token to miss")
	}

	// An order that never got a token must not be reachable with an empty hash.
	if _, err := model.GetOrderByViewToken(""); err == nil {
		t.Fatal("expected an empty token to miss")
	}
}
