package shop

import (
	"strconv"
	"strings"
	"sync"

	"thom-server/internal/data"
)

const (
	// OrderStatusPending is set when a Checkout Session is created.
	OrderStatusPending = "pending"
	// OrderStatusPaid is set once Stripe confirms payment.
	OrderStatusPaid = "paid"
	// OrderStatusRefundPending is set when a refund is requested but Stripe has
	// not confirmed it yet.
	OrderStatusRefundPending = "refund_pending"
	// OrderStatusRefunded is set once Stripe confirms the refund.
	OrderStatusRefunded = "refunded"
	// OrderStatusExpired is set when a Checkout Session expires before payment.
	OrderStatusExpired = "expired"
)

// OrderLine snapshots a listing's title and price at purchase time, so editing
// or deleting the listing can never rewrite order history.
type OrderLine struct {
	ID             string `json:"id"`
	ItemID         string `json:"itemId"`
	Title          string `json:"title"`
	UnitPriceCents int    `json:"unitPriceCents"`
	Quantity       int    `json:"quantity"`
}

// Order is a Stripe Checkout purchase. StockReserved marks an order whose
// stock was decremented when the session was created, and ExpiresAt is the
// unix timestamp its reservation hold ends; both are internal state and stay
// out of the API.
type Order struct {
	ID               string       `json:"id"`
	StripeSessionID  string       `json:"stripeSessionId"`
	Status           string       `json:"status"`
	CustomerEmail    string       `json:"customerEmail"`
	CustomerName     string       `json:"customerName"`
	ShippingAddress  string       `json:"shippingAddress"`
	ShipName         string       `json:"shipName"`
	ShipLine1        string       `json:"shipLine1"`
	ShipLine2        string       `json:"shipLine2"`
	ShipCity         string       `json:"shipCity"`
	ShipState        string       `json:"shipState"`
	ShipPostalCode   string       `json:"shipPostalCode"`
	ShipCountry      string       `json:"shipCountry"`
	AmountTotalCents int          `json:"amountTotalCents"`
	Currency         string       `json:"currency"`
	Lines            []*OrderLine `json:"lines"`
	RefundedAt       string       `json:"refundedAt"`
	RefundReason     string       `json:"refundReason"`
	CreatedAt        string       `json:"createdAt"`
	UpdatedAt        string       `json:"updatedAt"`
	StockReserved    int          `json:"-"`
	ExpiresAt        int64        `json:"-"`
}

// PaidDetails carries the customer and total information Stripe reports once a
// session is paid.
type PaidDetails struct {
	CustomerEmail    string
	CustomerName     string
	ShippingAddress  string
	ShipName         string
	ShipLine1        string
	ShipLine2        string
	ShipCity         string
	ShipState        string
	ShipPostalCode   string
	ShipCountry      string
	AmountTotalCents int
	Currency         string
	ViewTokenHash    string
}

// orderColumns is the full column list every single-order read scans. It is a
// constant so the session and view-token lookups cannot drift apart.
const orderColumns = `id, StripeSessionID, Status, CustomerEmail, CustomerName, ShippingAddress,
	ShipName, ShipLine1, ShipLine2, ShipCity, ShipState, ShipPostalCode, ShipCountry,
	AmountTotalCents, Currency, RefundedAt, RefundReason, CreatedAt, UpdatedAt,
	StockReserved, ExpiresAt`

// InsertPendingOrder records a Checkout Session and the lines the customer is
// paying for. D1 has no interactive transactions, so the order row and its
// lines are written as separate statements. A non-zero StockReserved with an
// ExpiresAt persists a checkout-time stock reservation; the zero values keep
// the legacy decrement-at-webhook behavior for pre-reservation orders.
func (m *Model) InsertPendingOrder(sessionID string, order *Order) (*Order, error) {
	result, err := m.DB.Exec(
		`INSERT INTO ShopOrders (StripeSessionID, Status, Currency, StockReserved, ExpiresAt)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionID,
		OrderStatusPending,
		currencyOr(order.Currency),
		order.StockReserved,
		order.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	for _, line := range order.Lines {
		lineResult, err := m.DB.Exec(
			`INSERT INTO ShopOrderLines (OrderID, ItemID, Title, UnitPriceCents, Quantity)
			 VALUES (?, ?, ?, ?, ?)`,
			orderID,
			line.ItemID,
			line.Title,
			line.UnitPriceCents,
			line.Quantity,
		)
		if err != nil {
			return nil, err
		}

		lineID, err := lineResult.LastInsertId()
		if err != nil {
			return nil, err
		}
		line.ID = strconv.FormatInt(lineID, 10)
	}

	order.ID = strconv.FormatInt(orderID, 10)
	order.StripeSessionID = sessionID
	order.Status = OrderStatusPending
	order.Currency = currencyOr(order.Currency)

	return order, nil
}

// GetOrderBySessionID looks an order up by its unguessable Stripe session id.
// Callers serving anonymous requests must redact the customer and shipping
// fields; the id alone is not authorization for personal data.
func (m *Model) GetOrderBySessionID(sessionID string) (*Order, error) {
	return m.getOrder("StripeSessionID = ?", sessionID)
}

// GetOrderByViewToken looks an order up by the hash of the magic-link token
// emailed to the buyer. The raw token is never stored. The caller treats the
// token as the credential, so this returns the full order.
func (m *Model) GetOrderByViewToken(tokenHash string) (*Order, error) {
	return m.getOrder("ViewTokenHash = ? AND ViewTokenHash <> ''", tokenHash)
}

// getOrder loads one order and its lines. where is a package constant clause,
// never caller input. The lines resolve the order id through a subquery so the
// two reads can run together instead of as two sequential D1 round trips.
func (m *Model) getOrder(where string, arg any) (*Order, error) {
	var (
		lines    []*OrderLine
		linesErr error
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lines, linesErr = m.getOrderLinesByOrder(where, arg)
	}()

	order, _, err := scanOrder(m.DB.QueryRow(
		`SELECT `+orderColumns+`
		 FROM ShopOrders
		 WHERE `+where,
		arg,
	))
	if err != nil {
		wg.Wait()
		return nil, data.NoRecord(err)
	}

	wg.Wait()
	if linesErr != nil {
		return nil, linesErr
	}
	order.Lines = lines

	return order, nil
}

// ListOrdersPage returns one page of orders newest first with their lines, for
// the admin view. cursor is 0 for the first page, otherwise it is the id of the
// last order on the previous page. nextCursor is 0 when there is no further
// page.
func (m *Model) ListOrdersPage(limit, cursor int) ([]*Order, int, error) {
	rows, err := m.DB.Query(
		`SELECT `+orderColumns+`
		 FROM ShopOrders
		 WHERE (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		cursor,
		cursor,
		limit+1,
	)
	if err != nil {
		return nil, 0, err
	}

	orders := make([]*Order, 0)
	orderIDs := make([]int, 0)
	for rows.Next() {
		order, orderID, err := scanOrder(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}

		orders = append(orders, order)
		orderIDs = append(orderIDs, orderID)
	}

	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}

	// Lines are loaded after the list cursor is closed: the pool can be a
	// single connection, so a nested query while rows are open would block.
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}

	// The extra row requested above proves another page exists; the cursor is
	// the id of the last order kept here.
	nextCursor := 0
	if len(orders) > limit {
		nextCursor = orderIDs[limit-1]
		orders = orders[:limit]
		orderIDs = orderIDs[:limit]
	}

	if len(orderIDs) == 0 {
		return orders, 0, nil
	}

	linesByOrder, err := m.loadOrderLines(orderIDs)
	if err != nil {
		return nil, 0, err
	}

	for index, orderID := range orderIDs {
		lines, ok := linesByOrder[orderID]
		if !ok {
			lines = make([]*OrderLine, 0)
		}
		orders[index].Lines = lines
	}

	return orders, nextCursor, nil
}

// scanOrder reads one row selected with orderColumns. The numeric id is
// returned separately because it is only ever exposed as a string.
func scanOrder(s scanner) (*Order, int, error) {
	order := &Order{}
	var orderID int

	err := s.Scan(
		&orderID,
		&order.StripeSessionID,
		&order.Status,
		&order.CustomerEmail,
		&order.CustomerName,
		&order.ShippingAddress,
		&order.ShipName,
		&order.ShipLine1,
		&order.ShipLine2,
		&order.ShipCity,
		&order.ShipState,
		&order.ShipPostalCode,
		&order.ShipCountry,
		&order.AmountTotalCents,
		&order.Currency,
		&order.RefundedAt,
		&order.RefundReason,
		&order.CreatedAt,
		&order.UpdatedAt,
		&order.StockReserved,
		&order.ExpiresAt,
	)
	if err != nil {
		return nil, 0, err
	}

	order.ID = strconv.Itoa(orderID)

	return order, orderID, nil
}

// loadOrderLines returns the lines of the given orders keyed by order id. The
// list cursor must already be closed: the pool can be a single connection, so a
// nested query while rows are open would block.
func (m *Model) loadOrderLines(orderIDs []int) (map[int][]*OrderLine, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(orderIDs)), ",")
	args := make([]any, len(orderIDs))
	for i, orderID := range orderIDs {
		args[i] = orderID
	}

	lineRows, err := m.DB.Query(
		`SELECT OrderID, id, ItemID, Title, UnitPriceCents, Quantity
		 FROM ShopOrderLines
		 WHERE OrderID IN (`+placeholders+`)
		 ORDER BY OrderID ASC, id ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer lineRows.Close()

	linesByOrder := make(map[int][]*OrderLine)
	for lineRows.Next() {
		line := &OrderLine{}
		var orderID, lineID int

		if err = lineRows.Scan(
			&orderID,
			&lineID,
			&line.ItemID,
			&line.Title,
			&line.UnitPriceCents,
			&line.Quantity,
		); err != nil {
			return nil, err
		}

		line.ID = strconv.Itoa(lineID)
		linesByOrder[orderID] = append(linesByOrder[orderID], line)
	}

	if err = lineRows.Err(); err != nil {
		return nil, err
	}

	return linesByOrder, nil
}

// getOrderLinesByOrder returns the lines of the order matching where/arg. where
// is a package constant clause, never caller input. It resolves the order id
// itself so it can run alongside the order read.
func (m *Model) getOrderLinesByOrder(where string, arg any) ([]*OrderLine, error) {
	rows, err := m.DB.Query(
		`SELECT id, ItemID, Title, UnitPriceCents, Quantity
		 FROM ShopOrderLines
		 WHERE OrderID = (SELECT id FROM ShopOrders WHERE `+where+`)
		 ORDER BY id ASC`,
		arg,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := make([]*OrderLine, 0)
	for rows.Next() {
		line := &OrderLine{}
		var lineID int

		if err = rows.Scan(
			&lineID,
			&line.ItemID,
			&line.Title,
			&line.UnitPriceCents,
			&line.Quantity,
		); err != nil {
			return nil, err
		}

		line.ID = strconv.Itoa(lineID)
		lines = append(lines, line)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
