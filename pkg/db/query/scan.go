package query

import (
	"gorm.io/gorm"
)

// ScanAll executes q and scans every result row into dest, like GORM's Scan. It exists because
// GORM v1.22.2 Scan ignores rows.Err() when reading the first row fails, so a context canceled
// after the query starts but before any row arrives yields empty results and a nil error.
func ScanAll(q *gorm.DB, dest any) error {
	rows, err := q.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		// ScanRows reads the remaining rows and reports rows.Err() for them itself.
		return q.ScanRows(rows, dest)
	}
	return rows.Err()
}
