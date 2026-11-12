package complianceeval

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// OpenDB connects to the project database from the PG_* environment (set by mise), as the other
// command-line tools do.
func OpenDB() (*sql.DB, error) {
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		get("PG_HOST", "/tmp"), get("PG_PORT", "5432"), get("PG_USER_NAME", "cding"), os.Getenv("PG_DB_NAME")))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot connect to the database (PG_DB_NAME=%q): %w", os.Getenv("PG_DB_NAME"), err)
	}
	return db, nil
}
