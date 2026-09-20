package coffee

import (
	"errors"

	"thom-server/internal/data"
)

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
	if cached, ok := m.cachedRoaster(roaster.Roaster); ok {
		return cached, nil
	}

	resolved, err := m.upsertRoasterUncached(roaster)
	if err != nil {
		return nil, err
	}

	m.storeRoaster(resolved.Roaster, resolved)
	return resolved, nil
}

func (m *Model) upsertRoasterUncached(roaster *Roaster) (*Roaster, error) {
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
