package coffee

import (
	"strconv"

	"thom-server/internal/data"
)

// entryColumns is the full column list every entry read scans. It is a constant
// so the list and single-entry reads cannot drift apart.
const entryColumns = `id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
	DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, GrinderID, Grinder,
	GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
	BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating, CreatedAt`

// GetAll returns every entry in full. The list view renders the same fields as
// the detail view, so returning summaries would only force a follow-up request
// per entry.
func (m *Model) GetAll() ([]*Entry, error) {
	stmt := `SELECT ` + entryColumns + `
		FROM CoffeeEntries
		ORDER BY BrewDate DESC, id DESC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*Entry, 0)
	for rows.Next() {
		entry, err := scanCoffeeEntry(rows)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (m *Model) GetByID(id int) (*Entry, error) {
	stmt := `SELECT ` + entryColumns + `
		FROM CoffeeEntries
		WHERE id = ?`

	entry, err := scanCoffeeEntry(m.DB.QueryRow(stmt, id))
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return entry, nil
}

func (m *Model) Insert(entry *Entry) (*Entry, error) {
	if err := m.resolveEntryLookups(entry); err != nil {
		return nil, err
	}

	stmt := `
		INSERT INTO CoffeeEntries (
			BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, GrinderID, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := m.DB.Exec(
		stmt,
		entry.Date,
		entry.CoffeeName,
		entry.Origin,
		entry.CoffeeVarietal,
		entry.ProcessingMethod,
		entry.DaysSinceRoast,
		entry.RoasterID,
		entry.Roaster,
		entry.BrewMethod,
		entry.Ratio,
		entry.GrinderID,
		entry.Grinder,
		entry.GrindSetting,
		entry.Dose,
		entry.YieldAmount,
		entry.WaterTemperature,
		entry.BrewTime,
		entry.BloomTime,
		entry.BloomWater,
		entry.PourNotes,
		entry.RoastLevel,
		entry.Notes,
		entry.Rating,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return m.GetByID(int(id))
}

func (m *Model) Update(id int, entry *Entry) (*Entry, error) {
	if err := m.resolveEntryLookups(entry); err != nil {
		return nil, err
	}

	stmt := `
		UPDATE CoffeeEntries
		SET BrewDate = ?,
			CoffeeName = ?,
			Origin = ?,
			CoffeeVarietal = ?,
			ProcessingMethod = ?,
			DaysSinceRoast = ?,
			RoasterID = ?,
			Roaster = ?,
			BrewMethod = ?,
			Ratio = ?,
			GrinderID = ?,
			Grinder = ?,
			GrindSetting = ?,
			Dose = ?,
			YieldAmount = ?,
			WaterTemperature = ?,
			BrewTime = ?,
			BloomTime = ?,
			BloomWater = ?,
			PourNotes = ?,
			RoastLevel = ?,
			Notes = ?,
			Rating = ?
		WHERE id = ?`

	result, err := m.DB.Exec(
		stmt,
		entry.Date,
		entry.CoffeeName,
		entry.Origin,
		entry.CoffeeVarietal,
		entry.ProcessingMethod,
		entry.DaysSinceRoast,
		entry.RoasterID,
		entry.Roaster,
		entry.BrewMethod,
		entry.Ratio,
		entry.GrinderID,
		entry.Grinder,
		entry.GrindSetting,
		entry.Dose,
		entry.YieldAmount,
		entry.WaterTemperature,
		entry.BrewTime,
		entry.BloomTime,
		entry.BloomWater,
		entry.PourNotes,
		entry.RoastLevel,
		entry.Notes,
		entry.Rating,
		id,
	)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, data.ErrNoRecord
	}

	return m.GetByID(id)
}

func (m *Model) Delete(id int) error {
	stmt := `
		DELETE FROM CoffeeEntries
		WHERE id = ?`

	result, err := m.DB.Exec(stmt, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return data.ErrNoRecord
	}

	return nil
}

// resolveEntryLookups registers an entry's roaster and grinder by name and
// points the entry at the stored rows, so an inserted or updated entry never
// holds a dangling lookup id.
func (m *Model) resolveEntryLookups(entry *Entry) error {
	roaster, err := m.upsertRoaster(&Roaster{ID: entry.RoasterID, Roaster: entry.Roaster})
	if err != nil {
		return err
	}
	entry.RoasterID = roaster.ID
	entry.Roaster = roaster.Roaster

	grinder, err := m.upsertGrinder(&Grinder{ID: entry.GrinderID, Grinder: entry.Grinder})
	if err != nil {
		return err
	}
	entry.GrinderID = grinder.ID
	entry.Grinder = grinder.Grinder

	return nil
}

func scanCoffeeEntry(s scanner) (*Entry, error) {
	entry := &Entry{}
	var id int

	err := s.Scan(
		&id,
		&entry.Date,
		&entry.CoffeeName,
		&entry.Origin,
		&entry.CoffeeVarietal,
		&entry.ProcessingMethod,
		&entry.DaysSinceRoast,
		&entry.RoasterID,
		&entry.Roaster,
		&entry.BrewMethod,
		&entry.Ratio,
		&entry.GrinderID,
		&entry.Grinder,
		&entry.GrindSetting,
		&entry.Dose,
		&entry.YieldAmount,
		&entry.WaterTemperature,
		&entry.BrewTime,
		&entry.BloomTime,
		&entry.BloomWater,
		&entry.PourNotes,
		&entry.RoastLevel,
		&entry.Notes,
		&entry.Rating,
		&entry.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	entry.ID = strconv.Itoa(id)
	entry.TastingNotes = entry.Notes

	return entry, nil
}
