package user

import "database/sql"

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(24) UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		avatar TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);`
	_, err := db.Exec(schema)
	return err
}
