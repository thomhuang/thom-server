package shop

import (
	"strconv"
	"strings"
)

// getMeasurements returns a listing's measurements in the order they were saved.
func (m *Model) getMeasurements(itemID int) ([]*Measurement, error) {
	rows, err := m.DB.Query(
		`SELECT id, Label, ValueInches
		 FROM ShopItemMeasurements
		 WHERE ItemID = ?
		 ORDER BY id ASC`,
		itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	measurements := make([]*Measurement, 0)
	for rows.Next() {
		measurement := &Measurement{}
		var id int

		if err = rows.Scan(&id, &measurement.Label, &measurement.ValueInches); err != nil {
			return nil, err
		}

		measurement.ID = strconv.Itoa(id)
		measurements = append(measurements, measurement)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return measurements, nil
}

// replaceMeasurements swaps a listing's whole measurement set for the given
// rows. D1 has no interactive transactions, so this deletes then inserts; the
// statements are independent, which is acceptable because the rows are derived
// from a request that is retried as a whole.
func (m *Model) replaceMeasurements(itemID int, measurements []*Measurement) error {
	if _, err := m.DB.Exec(`DELETE FROM ShopItemMeasurements WHERE ItemID = ?`, itemID); err != nil {
		return err
	}

	if len(measurements) == 0 {
		return nil
	}

	placeholders := make([]string, 0, len(measurements))
	args := make([]any, 0, len(measurements)*3)
	for _, measurement := range measurements {
		placeholders = append(placeholders, "(?, ?, ?)")
		args = append(args, itemID, measurement.Label, measurement.ValueInches)
	}

	stmt := `INSERT INTO ShopItemMeasurements (ItemID, Label, ValueInches) VALUES ` +
		strings.Join(placeholders, ", ")

	_, err := m.DB.Exec(stmt, args...)

	return err
}
