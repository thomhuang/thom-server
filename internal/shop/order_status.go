package shop

// MarkOrderPaid records a confirmed payment. Stripe retries webhooks
// aggressively, so the status guard makes this idempotent: only a pending
// order can become paid. A redelivery of a paid, refund_pending, or refunded
// order updates no rows and reports false, so it can never revive a refund.
func (m *Model) MarkOrderPaid(sessionID string, details PaidDetails) (bool, error) {
	return m.execChanged(
		`UPDATE ShopOrders
		 SET Status = ?,
			CustomerEmail = ?,
			CustomerName = ?,
			ShippingAddress = ?,
			ShipName = ?,
			ShipLine1 = ?,
			ShipLine2 = ?,
			ShipCity = ?,
			ShipState = ?,
			ShipPostalCode = ?,
			ShipCountry = ?,
			AmountTotalCents = ?,
			Currency = ?,
			ViewTokenHash = ?,
			UpdatedAt = datetime('now')
		 WHERE StripeSessionID = ? AND Status = ?`,
		OrderStatusPaid,
		details.CustomerEmail,
		details.CustomerName,
		details.ShippingAddress,
		details.ShipName,
		details.ShipLine1,
		details.ShipLine2,
		details.ShipCity,
		details.ShipState,
		details.ShipPostalCode,
		details.ShipCountry,
		details.AmountTotalCents,
		currencyOr(details.Currency),
		details.ViewTokenHash,
		sessionID,
		OrderStatusPending,
	)
}

// MarkOrderRefundPending records a refund request against a paid order. It only
// moves a paid order, so a repeated request updates nothing and reports false.
func (m *Model) MarkOrderRefundPending(sessionID, reason string) (bool, error) {
	return m.execChanged(
		`UPDATE ShopOrders
		 SET Status = ?, RefundReason = ?, UpdatedAt = datetime('now')
		 WHERE StripeSessionID = ? AND Status = ?`,
		OrderStatusRefundPending,
		reason,
		sessionID,
		OrderStatusPaid,
	)
}

// MarkOrderRefunded confirms a refund and stamps the time. It accepts a paid
// order (no refund request first) or one that is already refund_pending.
func (m *Model) MarkOrderRefunded(sessionID string) (bool, error) {
	return m.execChanged(
		`UPDATE ShopOrders
		 SET Status = ?, RefundedAt = datetime('now'), UpdatedAt = datetime('now')
		 WHERE StripeSessionID = ? AND Status IN (?, ?)`,
		OrderStatusRefunded,
		sessionID,
		OrderStatusPaid,
		OrderStatusRefundPending,
	)
}

// MarkOrderExpired records an expired Checkout Session. Only a pending order
// can expire, so a redelivered event or a race with payment updates nothing
// and reports false.
func (m *Model) MarkOrderExpired(sessionID string) (bool, error) {
	return m.execChanged(
		`UPDATE ShopOrders
		 SET Status = ?, UpdatedAt = datetime('now')
		 WHERE StripeSessionID = ? AND Status = ?`,
		OrderStatusExpired,
		sessionID,
		OrderStatusPending,
	)
}
