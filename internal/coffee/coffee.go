package coffee

import (
	"database/sql"
	"errors"
	"strconv"

	"thom-server/internal/data"
)

type Entry struct {
	ID               string `json:"id"`
	Date             string `json:"date"`
	CoffeeName       string `json:"coffeeName"`
	Origin           string `json:"origin"`
	CoffeeVarietal   string `json:"coffeeVarietal"`
	ProcessingMethod string `json:"processingMethod"`
	DaysSinceRoast   int     `json:"daysSinceRoast"`
	RoasterID        string  `json:"roasterId"`
	Roaster          string  `json:"roaster"`
	BrewMethod       string  `json:"brewMethod"`
	Ratio            string  `json:"ratio"`
	Grinder          string  `json:"grinder"`
	GrindSetting     float64 `json:"grindSetting"`
	Dose             int     `json:"dose"`
	YieldAmount      int     `json:"yieldAmount"`
	WaterTemperature int     `json:"waterTemperature"`
	BrewTime         string  `json:"brewTime"`
	BloomTime        string  `json:"bloomTime"`
	BloomWater       int     `json:"bloomWater"`
	PourNotes        string `json:"pourNotes"`
	RoastLevel       string `json:"roastLevel"`
	Notes            string `json:"notes"`
	TastingNotes     string `json:"tastingNotes,omitempty"`
	Rating           int    `json:"rating"`
	CreatedAt        string `json:"createdAt,omitempty"`
}

type EntrySummary struct {
	ID               string `json:"id"`
	Date             string `json:"date"`
	CoffeeName       string `json:"coffeeName"`
	Origin           string `json:"origin"`
	CoffeeVarietal   string `json:"coffeeVarietal"`
	ProcessingMethod string `json:"processingMethod"`
	Roaster          string `json:"roaster"`
	BrewMethod       string `json:"brewMethod"`
	Ratio            string `json:"ratio"`
	TastingNotes     string `json:"tastingNotes"`
	Rating           int    `json:"rating"`
}

type Roaster struct {
	ID        string `json:"id"`
	Roaster   string `json:"roaster"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Model struct {
	DB *sql.DB
}

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

		CREATE TABLE IF NOT EXISTS CoffeeRoasters (
			id TEXT PRIMARY KEY,
			Roaster TEXT NOT NULL COLLATE NOCASE UNIQUE,
			SortOrder INTEGER NOT NULL DEFAULT 1000,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS CoffeeRoastersSortIndex
		ON CoffeeRoasters(SortOrder ASC, Roaster COLLATE NOCASE ASC);`

	if _, err := m.DB.Exec(stmt); err != nil {
		return err
	}

	if err := m.ensureCoffeeEntryColumns(); err != nil {
		return err
	}

	return m.seedEntryRoasters()
}

func (m *Model) ensureCoffeeEntryColumns() error {
	rows, err := m.DB.Query(`PRAGMA table_info(CoffeeEntries)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)

		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		existingColumns[name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}

	columns := []struct {
		name       string
		definition string
	}{
		{name: "Origin", definition: "Origin TEXT NOT NULL DEFAULT ''"},
		{name: "CoffeeVarietal", definition: "CoffeeVarietal TEXT NOT NULL DEFAULT ''"},
		{name: "ProcessingMethod", definition: "ProcessingMethod TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if existingColumns[column.name] {
			continue
		}
		if _, err = m.DB.Exec("ALTER TABLE CoffeeEntries ADD COLUMN " + column.definition); err != nil {
			return err
		}
	}

	return nil
}

func (m *Model) seedEntryRoasters() error {
	stmt := `
		INSERT OR IGNORE INTO CoffeeRoasters (id, Roaster)
		SELECT RoasterID, Roaster
		FROM CoffeeEntries
		WHERE RoasterID != '' AND Roaster != ''`

	_, err := m.DB.Exec(stmt)
	return err
}

func (m *Model) GetRoasters() ([]*Roaster, error) {
	stmt := `
		SELECT id, Roaster, CreatedAt
		FROM CoffeeRoasters
		ORDER BY SortOrder ASC, Roaster COLLATE NOCASE ASC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roasters := make([]*Roaster, 0)
	for rows.Next() {
		roaster := &Roaster{}
		if err = rows.Scan(&roaster.ID, &roaster.Roaster, &roaster.CreatedAt); err != nil {
			return nil, err
		}

		roasters = append(roasters, roaster)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return roasters, nil
}

func (m *Model) UpsertRoaster(roaster *Roaster) (*Roaster, error) {
	existingRoaster, err := m.GetRoasterByName(roaster.Roaster)
	if err == nil {
		return existingRoaster, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	existingRoaster, err = m.GetRoasterByID(roaster.ID)
	if err == nil {
		return existingRoaster, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	stmt := `
		INSERT INTO CoffeeRoasters (id, Roaster)
		VALUES (?, ?)`

	if _, err = m.DB.Exec(stmt, roaster.ID, roaster.Roaster); err != nil {
		return nil, err
	}

	return m.GetRoasterByID(roaster.ID)
}

func (m *Model) GetRoasterByName(name string) (*Roaster, error) {
	stmt := `
		SELECT id, Roaster, CreatedAt
		FROM CoffeeRoasters
		WHERE Roaster = ? COLLATE NOCASE`

	roaster := &Roaster{}
	err := m.DB.QueryRow(stmt, name).Scan(&roaster.ID, &roaster.Roaster, &roaster.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	return roaster, nil
}

func (m *Model) GetRoasterByID(id string) (*Roaster, error) {
	stmt := `
		SELECT id, Roaster, CreatedAt
		FROM CoffeeRoasters
		WHERE id = ?`

	roaster := &Roaster{}
	err := m.DB.QueryRow(stmt, id).Scan(&roaster.ID, &roaster.Roaster, &roaster.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	return roaster, nil
}

func (m *Model) GetAll() ([]*EntrySummary, error) {
	stmt := `
		SELECT id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			Roaster, BrewMethod, Ratio, Notes, Rating
		FROM CoffeeEntries
		ORDER BY BrewDate DESC, id DESC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*EntrySummary, 0)
	for rows.Next() {
		entry := &EntrySummary{}
		var id int
		if err = rows.Scan(
			&id,
			&entry.Date,
			&entry.CoffeeName,
			&entry.Origin,
			&entry.CoffeeVarietal,
			&entry.ProcessingMethod,
			&entry.Roaster,
			&entry.BrewMethod,
			&entry.Ratio,
			&entry.TastingNotes,
			&entry.Rating,
		); err != nil {
			return nil, err
		}

		entry.ID = strconv.Itoa(id)
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (m *Model) GetByID(id int) (*Entry, error) {
	stmt := `
		SELECT id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating, CreatedAt
		FROM CoffeeEntries
		WHERE id = ?`

	entry, err := scanCoffeeEntry(m.DB.QueryRow(stmt, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	return entry, nil
}

func (m *Model) Insert(entry *Entry) (*Entry, error) {
	roaster, err := m.UpsertRoaster(&Roaster{ID: entry.RoasterID, Roaster: entry.Roaster})
	if err != nil {
		return nil, err
	}
	entry.RoasterID = roaster.ID
	entry.Roaster = roaster.Roaster

	stmt := `
		INSERT INTO CoffeeEntries (
			BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
	roaster, err := m.UpsertRoaster(&Roaster{ID: entry.RoasterID, Roaster: entry.Roaster})
	if err != nil {
		return nil, err
	}
	entry.RoasterID = roaster.ID
	entry.Roaster = roaster.Roaster

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

type scanner interface {
	Scan(dest ...any) error
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
