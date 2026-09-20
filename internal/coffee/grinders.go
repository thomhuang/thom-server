package coffee

import (
	"errors"

	"thom-server/internal/data"
)

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
