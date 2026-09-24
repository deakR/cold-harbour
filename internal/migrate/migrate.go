package migrate

import (
	"database/sql"

	"coldharbour/migrations"

	"github.com/pressly/goose/v3"
)

func Up(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := baseline(db); err != nil {
		return err
	}
	return goose.Up(db, ".")
}

func baseline(db *sql.DB) error {
	var tenants, versionTable bool
	if err := db.QueryRow(`SELECT to_regclass('public.tenants') IS NOT NULL`).Scan(&tenants); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&versionTable); err != nil {
		return err
	}
	if !tenants || versionTable {
		return nil
	}
	if _, err := goose.EnsureDBVersion(db); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (1, true)`)
	return err
}
