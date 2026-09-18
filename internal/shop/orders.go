package shop

import (
	"database/sql"
	"errors"
	"strconv"

	"thom-server/internal/data"
)

const (
	// OrderStatusPending is set when a Checkout Session is created.
	OrderStatusPending = "pending"
	// OrderStatusPaid is set once Stripe confirms payment.
	OrderStatusPaid = "paid"
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

// Order is a Stripe Checkout purchase.
type Order struct {
	ID               string       `json:"id"`
	StripeSessionID  string       `json:"stripeSessionId"`
	Status           string       `json:"status"`
	CustomerEmail    string       `json:"customerEmail"`
	CustomerName     string       `json:"customerName"`
	ShippingAddress  string       `json:"shippingAddress"`
	AmountTotalCents int          `json:"amountTotalCents"`
	Currency         string       `json:"currency"`
	Lines            []*OrderLine `json:"lines"`
	CreatedAt        string       `json:"createdAt"`
	UpdatedAt        string       `json:"updatedAt"`
}

// PaidDetails carries the customer and total information Stripe reports once a
// session is paid.
type PaidDetails struct {
	CustomerEmail    string
	CustomerName     string
	ShippingAddress  string
	AmountTotalCents int
	Currency         string
}

// InsertPendingOrder records a Checkout Session and the lines the customer is
// paying for. D1 has no interactive transactions, so the order row and its
// lines are written as separate statements.
func (m *Model) InsertPendingOrder(sessionID string, order *Order) (*Order, error) {
	result, err := m.DB.Exec(
		`INSERT INTO ShopOrders (StripeSessionID, Status, Currency) VALUES (?, ?, ?)`,
		sessionID,
		OrderStatusPending,
		currencyOr(order.Currency),
	)
	if err != nil {
		return nil, err
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	for _, line := range order.Lines {
		if _, err = m.DB.Exec(
			`INSERT INTO ShopOrderLines (OrderID, ItemID, Title, UnitPriceCents, Quantity)
			 VALUES (?, ?, ?, ?, ?)`,
			orderID,
			line.ItemID,
			line.Title,
			line.UnitPriceCents,
			line.Quantity,
		); err != nil {
			return nil, err
		}
	}

	return m.GetOrderBySessionID(sessionID)
}

// GetOrderBySessionID looks an order up by its unguessable Stripe session id,
// which is safe to use as a public lookup key.
func (m *Model) GetOrderBySessionID(sessionID string) (*Order, error) {
	order := &Order{}
	var orderID int

	err := m.DB.QueryRow(
		`SELECT id, StripeSessionID, Status, CustomerEmail, CustomerName,
			ShippingAddress, AmountTotalCents, Currency, CreatedAt, UpdatedAt
		 FROM ShopOrders
		 WHERE StripeSessionID = ?`,
		sessionID,
	).Scan(
		&orderID,
		&order.StripeSessionID,
		&order.Status,
		&order.CustomerEmail,
		&order.CustomerName,
		&order.ShippingAddress,
		&order.AmountTotalCents,
		&order.Currency,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	order.ID = strconv.Itoa(orderID)

	lines, err := m.getOrderLines(orderID)
	if err != nil {
		return nil, err
	}
	order.Lines = lines

	return order, nil
}

// ListOrders returns orders newest first for the admin view. Lines are loaded
// separately by GetOrderBySessionID to keep the list query flat.
func (m *Model) ListOrders() ([]*Order, error) {
	rows, err := m.DB.Query(
		`SELECT id, StripeSessionID, Status, CustomerEmail, CustomerName,
			ShippingAddress, AmountTotalCents, Currency, CreatedAt, UpdatedAt
		 FROM ShopOrders
		 ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]*Order, 0)
	for rows.Next() {
		order := &Order{}
		var orderID int

		if err = rows.Scan(
			&orderID,
			&order.StripeSessionID,
			&order.Status,
			&order.CustomerEmail,
			&order.CustomerName,
			&order.ShippingAddress,
			&order.AmountTotalCents,
			&order.Currency,
			&order.CreatedAt,
			&order.UpdatedAt,
		); err != nil {
			return nil, err
		}

		order.ID = strconv.Itoa(orderID)
		orders = append(orders, order)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

// MarkOrderPaid records a confirmed payment. Stripe retries webhooks
// aggressively, so the status guard makes this idempotent: a repeated delivery
// updates no rows and reports false.
func (m *Model) MarkOrderPaid(sessionID string, details PaidDetails) (bool, error) {
	result, err := m.DB.Exec(
		`UPDATE ShopOrders
		 SET Status = ?,
			CustomerEmail = ?,
			CustomerName = ?,
			ShippingAddress = ?,
			AmountTotalCents = ?,
			Currency = ?,
			UpdatedAt = datetime('now')
		 WHERE StripeSessionID = ? AND Status <> ?`,
		OrderStatusPaid,
		details.CustomerEmail,
		details.CustomerName,
		details.ShippingAddress,
		details.AmountTotalCents,
		currencyOr(details.Currency),
		sessionID,
		OrderStatusPaid,
	)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

// DecrementStock lowers a listing's stock only when enough remains. D1 has no
// interactive transactions, so the conditional UPDATE plus RowsAffected check
// is what prevents overselling. It reports whether the stock was available.
func (m *Model) DecrementStock(itemID string, quantity int) (bool, error) {
	if quantity < 1 {
		return false, nil
	}

	result, err := m.DB.Exec(
		`UPDATE ShopItems SET Stock = Stock - ? WHERE id = ? AND Stock >= ?`,
		quantity,
		itemID,
		quantity,
	)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

func (m *Model) getOrderLines(orderID int) ([]*OrderLine, error) {
	rows, err := m.DB.Query(
		`SELECT id, ItemID, Title, UnitPriceCents, Quantity
		 FROM ShopOrderLines
		 WHERE OrderID = ?
		 ORDER BY id ASC`,
		orderID,
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
