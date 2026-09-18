package data

import (
	"database/sql"
	"errors"
)

var ErrNoRecord = errors.New("data: no matching record found")

// NoRecord maps sql.ErrNoRows onto ErrNoRecord and passes every other error
// through unchanged.
func NoRecord(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoRecord
	}
	return err
}
