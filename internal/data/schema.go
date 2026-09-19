package data

import "database/sql"

// Column is a column EnsureColumns adds to a table when it is missing.
// Definition is the full ADD COLUMN clause, for example a name, type, and
// default, without the "ALTER TABLE ... ADD COLUMN" prefix.
type Column struct {
	Name       string
	Definition string
}

// EnsureColumns adds any of the given columns that the table does not already
// have. Table names and column definitions are package constants, not user
// input, so they can be concatenated into the statements.
func EnsureColumns(db *sql.DB, table string, columns []Column) error {
	existingColumns, err := tableColumns(db, table)
	if err != nil {
		return err
	}

	for _, column := range columns {
		if existingColumns[column.Name] {
			continue
		}
		if _, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column.Definition); err != nil {
			return err
		}
	}

	return nil
}

// DropColumns removes each named column that the table still has. It is the
// mirror of EnsureColumns, used to retire a column once its data has moved
// elsewhere. Table and column names are package constants, not user input.
func DropColumns(db *sql.DB, table string, columns ...string) error {
	existingColumns, err := tableColumns(db, table)
	if err != nil {
		return err
	}

	for _, name := range columns {
		if !existingColumns[name] {
			continue
		}
		if _, err = db.Exec("ALTER TABLE " + table + " DROP COLUMN " + name); err != nil {
			return err
		}
	}

	return nil
}

// tableColumns returns the set of column names a table currently has.
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
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
			return nil, err
		}
		columns[name] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}
