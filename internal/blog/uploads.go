package blog

import (
	"strings"
	"time"
)

// uploadTTL is how long a pasted image may sit in R2 before a saved post must
// reference it. The sweeper deletes uploads older than this that were never
// committed, so an abandoned post does not leak objects forever.
const uploadTTL = 24 * time.Hour

// AddUpload records an object key the browser was just presigned to upload. It
// is removed either when a post body references the key (CommitUploads) or when
// the sweeper expires it.
func (m *Model) AddUpload(objectKey string) error {
	_, err := m.DB.Exec(
		`INSERT INTO BlogUploads (ObjectKey, CreatedAt) VALUES (?, ?)`,
		objectKey,
		time.Now().Unix(),
	)
	return err
}

// DeleteUploads removes pending uploads for the given keys. It is used both to
// commit keys a post now references and to drop keys the sweeper expired.
func (m *Model) DeleteUploads(keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args := make([]any, len(keys))
	for index, key := range keys {
		args[index] = key
	}

	_, err := m.DB.Exec(
		`DELETE FROM BlogUploads WHERE ObjectKey IN (`+placeholders+`)`,
		args...,
	)
	return err
}

// ExpiredUploads returns the keys of pending uploads older than the TTL.
func (m *Model) ExpiredUploads(now time.Time) ([]string, error) {
	cutoff := now.Add(-uploadTTL).Unix()

	rows, err := m.DB.Query(
		`SELECT ObjectKey FROM BlogUploads WHERE CreatedAt < ?`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}
