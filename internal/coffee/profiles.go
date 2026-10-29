package coffee

import (
	"thom-server/internal/data"
)

func (m *Model) GetProfiles() ([]*Profile, error) {
	stmt := `
		SELECT id, Name, BrewMethod, Notes, CreatedAt
		FROM CoffeeProfiles
		ORDER BY Name COLLATE NOCASE ASC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := make([]*Profile, 0)
	for rows.Next() {
		profile := &Profile{}
		if err = rows.Scan(&profile.ID, &profile.Name, &profile.BrewMethod, &profile.Notes, &profile.CreatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (m *Model) GetProfileByID(id string) (*Profile, error) {
	return getProfileByID(m.DB, id)
}

func (m *Model) GetProfileByName(name string) (*Profile, error) {
	return getProfileByName(m.DB, name)
}

func getProfileByID(q querier, id string) (*Profile, error) {
	stmt := `
		SELECT id, Name, BrewMethod, Notes, CreatedAt
		FROM CoffeeProfiles
		WHERE id = ?`

	profile := &Profile{}
	err := q.QueryRow(stmt, id).Scan(&profile.ID, &profile.Name, &profile.BrewMethod, &profile.Notes, &profile.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return profile, nil
}

func getProfileByName(q querier, name string) (*Profile, error) {
	stmt := `
		SELECT id, Name, BrewMethod, Notes, CreatedAt
		FROM CoffeeProfiles
		WHERE Name = ? COLLATE NOCASE`

	profile := &Profile{}
	err := q.QueryRow(stmt, name).Scan(&profile.ID, &profile.Name, &profile.BrewMethod, &profile.Notes, &profile.CreatedAt)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return profile, nil
}

func (m *Model) InsertProfile(profile *Profile) (*Profile, error) {
	stmt := `
		INSERT INTO CoffeeProfiles (id, Name, BrewMethod, Notes)
		VALUES (?, ?, ?, ?)`

	if _, err := m.DB.Exec(stmt, profile.ID, profile.Name, profile.BrewMethod, profile.Notes); err != nil {
		return nil, err
	}

	return getProfileByID(m.DB, profile.ID)
}

func (m *Model) UpdateProfile(id string, profile *Profile) (*Profile, error) {
	stmt := `
		UPDATE CoffeeProfiles
		SET Name = ?, BrewMethod = ?, Notes = ?
		WHERE id = ?`

	result, err := m.DB.Exec(stmt, profile.Name, profile.BrewMethod, profile.Notes, id)
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

	return getProfileByID(m.DB, id)
}

func (m *Model) DeleteProfile(id string) error {
	// Clear the reference first so a deleted profile never leaves an entry with
	// a dangling profileId that would fail later profile validation.
	if _, err := m.DB.Exec(
		`UPDATE CoffeeEntries SET ProfileID = '' WHERE ProfileID = ?`,
		id,
	); err != nil {
		return err
	}

	stmt := `
		DELETE FROM CoffeeProfiles
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
