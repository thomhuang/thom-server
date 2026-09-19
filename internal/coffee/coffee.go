package coffee

import (
	"database/sql"
	"errors"
	"strconv"

	"thom-server/internal/data"
)

type Entry struct {
	ID               string  `json:"id"`
	Date             string  `json:"date"`
	CoffeeName       string  `json:"coffeeName"`
	Origin           string  `json:"origin"`
	CoffeeVarietal   string  `json:"coffeeVarietal"`
	ProcessingMethod string  `json:"processingMethod"`
	DaysSinceRoast   int     `json:"daysSinceRoast"`
	RoasterID        string  `json:"roasterId"`
	Roaster          string  `json:"roaster"`
	BrewMethod       string  `json:"brewMethod"`
	Ratio            string  `json:"ratio"`
	GrinderID        string  `json:"grinderId"`
	Grinder          string  `json:"grinder"`
	GrindSetting     float64 `json:"grindSetting"`
	Dose             int     `json:"dose"`
	YieldAmount      int     `json:"yieldAmount"`
	WaterTemperature int     `json:"waterTemperature"`
	BrewTime         string  `json:"brewTime"`
	BloomTime        string  `json:"bloomTime"`
	BloomWater       int     `json:"bloomWater"`
	PourNotes        string  `json:"pourNotes"`
	RoastLevel       string  `json:"roastLevel"`
	Notes            string  `json:"notes"`
	TastingNotes     string  `json:"tastingNotes,omitempty"`
	Rating           int     `json:"rating"`
	CreatedAt        string  `json:"createdAt,omitempty"`
}

type Roaster struct {
	ID        string `json:"id"`
	Roaster   string `json:"roaster"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Grinder struct {
	ID        string `json:"id"`
	Grinder   string `json:"grinder"`
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
	return m.upsertRoaster(roaster)
}

func (m *Model) upsertRoaster(roaster *Roaster) (*Roaster, error) {
	existingRoaster, err := getRoasterByName(m.DB, roaster.Roaster)
	if err == nil {
		return existingRoaster, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	if roaster.ID == "" {
		roaster.ID = SlugifyName(roaster.Roaster)
	}
	if roaster.ID == "" {
		return nil, data.ErrNoRecord
	}

	existingRoaster, err = getRoasterByID(m.DB, roaster.ID)
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

	return getRoasterByID(m.DB, roaster.ID)
}

func (m *Model) GetRoasterByName(name string) (*Roaster, error) {
	return getRoasterByName(m.DB, name)
}

func getRoasterByName(q querier, name string) (*Roaster, error) {
	stmt := `
		SELECT id, Roaster, CreatedAt
		FROM CoffeeRoasters
		WHERE Roaster = ? COLLATE NOCASE`

	roaster := &Roaster{}
	err := q.QueryRow(stmt, name).Scan(&roaster.ID, &roaster.Roaster, &roaster.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return roaster, nil
}

func (m *Model) GetRoasterByID(id string) (*Roaster, error) {
	return getRoasterByID(m.DB, id)
}

func getRoasterByID(q querier, id string) (*Roaster, error) {
	stmt := `
		SELECT id, Roaster, CreatedAt
		FROM CoffeeRoasters
		WHERE id = ?`

	roaster := &Roaster{}
	err := q.QueryRow(stmt, id).Scan(&roaster.ID, &roaster.Roaster, &roaster.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return roaster, nil
}

func (m *Model) GetGrinders() ([]*Grinder, error) {
	stmt := `
		SELECT id, Grinder, CreatedAt
		FROM CoffeeGrinders
		ORDER BY SortOrder ASC, Grinder COLLATE NOCASE ASC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grinders := make([]*Grinder, 0)
	for rows.Next() {
		grinder := &Grinder{}
		if err = rows.Scan(&grinder.ID, &grinder.Grinder, &grinder.CreatedAt); err != nil {
			return nil, err
		}

		grinders = append(grinders, grinder)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return grinders, nil
}

func (m *Model) UpsertGrinder(grinder *Grinder) (*Grinder, error) {
	return m.upsertGrinder(grinder)
}

func (m *Model) upsertGrinder(grinder *Grinder) (*Grinder, error) {
	existingGrinder, err := getGrinderByName(m.DB, grinder.Grinder)
	if err == nil {
		return existingGrinder, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	if grinder.ID == "" {
		grinder.ID = SlugifyName(grinder.Grinder)
	}
	if grinder.ID == "" {
		return nil, data.ErrNoRecord
	}

	existingGrinder, err = getGrinderByID(m.DB, grinder.ID)
	if err == nil {
		return existingGrinder, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	stmt := `
		INSERT INTO CoffeeGrinders (id, Grinder)
		VALUES (?, ?)`

	if _, err = m.DB.Exec(stmt, grinder.ID, grinder.Grinder); err != nil {
		return nil, err
	}

	return getGrinderByID(m.DB, grinder.ID)
}

func (m *Model) GetGrinderByName(name string) (*Grinder, error) {
	return getGrinderByName(m.DB, name)
}

func getGrinderByName(q querier, name string) (*Grinder, error) {
	stmt := `
		SELECT id, Grinder, CreatedAt
		FROM CoffeeGrinders
		WHERE Grinder = ? COLLATE NOCASE`

	grinder := &Grinder{}
	err := q.QueryRow(stmt, name).Scan(&grinder.ID, &grinder.Grinder, &grinder.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return grinder, nil
}

func (m *Model) GetGrinderByID(id string) (*Grinder, error) {
	return getGrinderByID(m.DB, id)
}

func getGrinderByID(q querier, id string) (*Grinder, error) {
	stmt := `
		SELECT id, Grinder, CreatedAt
		FROM CoffeeGrinders
		WHERE id = ?`

	grinder := &Grinder{}
	err := q.QueryRow(stmt, id).Scan(&grinder.ID, &grinder.Grinder, &grinder.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return grinder, nil
}

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
	roaster, err := m.upsertRoaster(&Roaster{ID: entry.RoasterID, Roaster: entry.Roaster})
	if err != nil {
		return nil, err
	}
	entry.RoasterID = roaster.ID
	entry.Roaster = roaster.Roaster

	grinder, err := m.upsertGrinder(&Grinder{ID: entry.GrinderID, Grinder: entry.Grinder})
	if err != nil {
		return nil, err
	}
	entry.GrinderID = grinder.ID
	entry.Grinder = grinder.Grinder

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
	roaster, err := m.upsertRoaster(&Roaster{ID: entry.RoasterID, Roaster: entry.Roaster})
	if err != nil {
		return nil, err
	}
	entry.RoasterID = roaster.ID
	entry.Roaster = roaster.Roaster

	grinder, err := m.upsertGrinder(&Grinder{ID: entry.GrinderID, Grinder: entry.Grinder})
	if err != nil {
		return nil, err
	}
	entry.GrinderID = grinder.ID
	entry.Grinder = grinder.Grinder

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

type scanner interface {
	Scan(dest ...any) error
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
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

// SlugifyName turns a display name into the stable id used by the roaster and
// grinder lookups: lowercase alphanumerics separated by single dashes.
func SlugifyName(value string) string {
	return data.SlugifyName(value)
}
