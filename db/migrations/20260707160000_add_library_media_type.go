package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddLibraryMediaType, downAddLibraryMediaType)
}

func upAddLibraryMediaType(ctx context.Context, tx *sql.Tx) error {
	// Check if column already exists to make migration idempotent
	var count int
	row := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('library') WHERE name = 'media_type'`)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // Column already exists, skip
	}
	_, err := tx.ExecContext(ctx, `ALTER TABLE library ADD COLUMN media_type VARCHAR(20) DEFAULT 'music'`)
	return err
}

func downAddLibraryMediaType(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't support DROP COLUMN directly
	return nil
}
