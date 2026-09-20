package coffee

import (
	"thom-server/internal/data"
)

func (m *Model) EnsureSchema() error {
	stmt := `
		CREATE TABLE IF NOT EXISTS CoffeeEntries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			BrewDate TEXT NOT NULL,
			CoffeeName TEXT NOT NULL,
			Origin TEXT NOT NULL DEFAULT '',
			CoffeeVarietal TEXT NOT NULL DEFAULT '',
			ProcessingMethod TEXT NOT NULL DEFAULT '',
			DaysSinceRoast INTEGER NOT NULL DEFAULT 0,
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting REAL NOT NULL DEFAULT 0.0,
			Dose INTEGER NOT NULL DEFAULT 0,
			YieldAmount INTEGER NOT NULL DEFAULT 0,
			WaterTemperature INTEGER NOT NULL DEFAULT 0,
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater INTEGER NOT NULL DEFAULT 0,
			PourNotes TEXT NOT NULL DEFAULT '',
			RoastLevel TEXT NOT NULL DEFAULT '',
			Notes TEXT NOT NULL,
			Rating INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS CoffeeEntriesBrewDateIndex
		ON CoffeeEntries(BrewDate DESC, id DESC);

		CREATE INDEX IF NOT EXISTS CoffeeEntriesRoasterIndex
		ON CoffeeEntries(RoasterID);

		CREATE INDEX IF NOT EXISTS CoffeeEntriesBrewMethodIndex
		ON CoffeeEntries(BrewMethod);

		CREATE TABLE IF NOT EXISTS CoffeeRoasters (
			id TEXT PRIMARY KEY,
			Roaster TEXT NOT NULL COLLATE NOCASE UNIQUE,
			SortOrder INTEGER NOT NULL DEFAULT 1000,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS CoffeeRoastersSortIndex
		ON CoffeeRoasters(SortOrder ASC, Roaster COLLATE NOCASE ASC);

		CREATE TABLE IF NOT EXISTS CoffeeGrinders (
			id TEXT PRIMARY KEY,
			Grinder TEXT NOT NULL COLLATE NOCASE UNIQUE,
			SortOrder INTEGER NOT NULL DEFAULT 1000,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS CoffeeGrindersSortIndex
		ON CoffeeGrinders(SortOrder ASC, Grinder COLLATE NOCASE ASC);`

	if _, err := m.DB.Exec(stmt); err != nil {
		return err
	}

	if err := m.ensureCoffeeEntryColumns(); err != nil {
		return err
	}

	// GrinderID is added by ensureCoffeeEntryColumns, so its index can only be
	// created after the column exists.
	if _, err := m.DB.Exec(
		`CREATE INDEX IF NOT EXISTS CoffeeEntriesGrinderIndex ON CoffeeEntries(GrinderID)`,
	); err != nil {
		return err
	}

	if err := m.backfillEntryGrinders(); err != nil {
		return err
	}

	return m.seedEntryRoastersAndGrinders()
}

func (m *Model) ensureCoffeeEntryColumns() error {
	return data.EnsureColumns(m.DB, "CoffeeEntries", []data.Column{
		{Name: "Origin", Definition: "Origin TEXT NOT NULL DEFAULT ''"},
		{Name: "CoffeeVarietal", Definition: "CoffeeVarietal TEXT NOT NULL DEFAULT ''"},
		{Name: "ProcessingMethod", Definition: "ProcessingMethod TEXT NOT NULL DEFAULT ''"},
		{Name: "GrinderID", Definition: "GrinderID TEXT NOT NULL DEFAULT ''"},
	})
}

// backfillEntryGrinders gives legacy rows a GrinderID derived from their
// free-text Grinder value and registers each distinct grinder in the lookup.
func (m *Model) backfillEntryGrinders() error {
	rows, err := m.DB.Query(
		`SELECT DISTINCT Grinder FROM CoffeeEntries WHERE GrinderID = '' AND Grinder != ''`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		return err
	}

	for _, name := range names {
		id := SlugifyName(name)
		if id == "" {
			continue
		}
		if _, err = m.DB.Exec(
			`INSERT OR IGNORE INTO CoffeeGrinders (id, Grinder) VALUES (?, ?)`,
			id,
			name,
		); err != nil {
			return err
		}

		// The name may already exist under a different id, so resolve the row
		// that actually holds the name before pointing entries at it.
		existingGrinder, err := getGrinderByName(m.DB, name)
		if err != nil {
			return err
		}
		if _, err = m.DB.Exec(
			`UPDATE CoffeeEntries SET GrinderID = ?
			 WHERE GrinderID = '' AND Grinder = ? COLLATE NOCASE`,
			existingGrinder.ID,
			name,
		); err != nil {
			return err
		}
	}

	return nil
}

func (m *Model) seedEntryRoastersAndGrinders() error {
	roasters := `
		INSERT OR IGNORE INTO CoffeeRoasters (id, Roaster)
		SELECT RoasterID, Roaster
		FROM CoffeeEntries
		WHERE RoasterID != '' AND Roaster != ''`
	if _, err := m.DB.Exec(roasters); err != nil {
		return err
	}

	grinders := `
		INSERT OR IGNORE INTO CoffeeGrinders (id, Grinder)
		SELECT GrinderID, Grinder
		FROM CoffeeEntries
		WHERE GrinderID != '' AND Grinder != ''`

	_, err := m.DB.Exec(grinders)
	return err
}
