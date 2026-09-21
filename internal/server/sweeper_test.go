package server

import (
	"context"
	"strconv"
	"testing"
	"time"

	shop "thom-server/internal/shop"
)

func TestRunReservationSweeperReleasesExpiredHolds(t *testing.T) {
	app := newTestApp(t)

	item, err := app.shop.InsertItem(&shop.Item{
		Title:       "Held item",
		PriceCents:  1000,
		Currency:    "usd",
		Stock:       1,
		IsPublished: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := app.shop.DecrementStock(item.ID, 1); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := app.shop.InsertPendingOrder("cs_sweep", &shop.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(-time.Minute).Unix(),
		Lines:         []*shop.OrderLine{{ItemID: item.ID, Title: item.Title, UnitPriceCents: item.PriceCents, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	// A cancelled context sweeps once and returns instead of waiting a tick.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.RunReservationSweeper(ctx)

	order, err := app.shop.GetOrderBySessionID("cs_sweep")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shop.OrderStatusExpired {
		t.Fatalf("status = %q, want expired after the sweep", order.Status)
	}

	itemID, err := strconv.Atoi(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := app.shop.GetItemByID(itemID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Stock != 1 {
		t.Fatalf("stock = %d, want 1 after the sweep restored it", reloaded.Stock)
	}
}
