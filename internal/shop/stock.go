package shop

// ReleaseExpiredReservations restores the stock held by reservations whose
// hold window has passed and marks their orders expired. It is the lazy sweep
// behind the checkout.session.expired webhook, so a missed webhook delivery
// can never strand stock. Each order is marked before its stock is restored:
// whoever wins that status transition owns the stock, so a concurrent payment
// or webhook can never restore and decrement the same reservation twice.
func (m *Model) ReleaseExpiredReservations(now int64) error {
	sessionIDs, err := m.expiredReservationSessionIDs(now)
	if err != nil {
		return err
	}

	for _, sessionID := range sessionIDs {
		if err = m.releaseReservation(sessionID); err != nil {
			return err
		}
	}

	return nil
}

// expiredReservationSessionIDs returns the pending, stock-reserving orders
// whose hold has passed.
func (m *Model) expiredReservationSessionIDs(now int64) ([]string, error) {
	rows, err := m.DB.Query(
		`SELECT StripeSessionID
		 FROM ShopOrders
		 WHERE Status = ? AND StockReserved = 1 AND ExpiresAt > 0 AND ExpiresAt <= ?`,
		OrderStatusPending,
		now,
	)
	if err != nil {
		return nil, err
	}

	sessionIDs := make([]string, 0)
	for rows.Next() {
		var sessionID string
		if err = rows.Scan(&sessionID); err != nil {
			rows.Close()
			return nil, err
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	return sessionIDs, nil
}

// releaseReservation marks one expired-pending order expired and restores the
// stock it reserved. MarkOrderExpired only moves a pending order, so a race
// with payment or a redelivered event restores stock exactly once.
func (m *Model) releaseReservation(sessionID string) error {
	order, err := m.getOrder("StripeSessionID = ?", sessionID)
	if err != nil {
		return err
	}

	changed, err := m.MarkOrderExpired(sessionID)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	for _, line := range order.Lines {
		if err = m.RestoreStock(line.ItemID, line.Quantity); err != nil {
			return err
		}
	}

	return nil
}

// RestoreStock puts a refunded or cancelled line's quantity back on a listing.
// A non-positive quantity is a no-op.
func (m *Model) RestoreStock(itemID string, quantity int) error {
	if quantity < 1 {
		return nil
	}

	_, err := m.DB.Exec(
		`UPDATE ShopItems SET Stock = Stock + ? WHERE id = ?`,
		quantity,
		itemID,
	)

	return err
}

// DecrementStock lowers a listing's stock only when enough remains. D1 has no
// interactive transactions, so the conditional UPDATE plus RowsAffected check
// is what prevents overselling. It reports whether the stock was available.
func (m *Model) DecrementStock(itemID string, quantity int) (bool, error) {
	if quantity < 1 {
		return false, nil
	}

	return m.execChanged(
		`UPDATE ShopItems SET Stock = Stock - ? WHERE id = ? AND Stock >= ?`,
		quantity,
		itemID,
		quantity,
	)
}
