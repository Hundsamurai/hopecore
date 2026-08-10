package store

import "time"

// nowUTC используется gorm для CreatedAt/UpdatedAt.
func nowUTC() time.Time {
	return time.Now().UTC()
}
