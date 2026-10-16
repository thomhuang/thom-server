package shop

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
