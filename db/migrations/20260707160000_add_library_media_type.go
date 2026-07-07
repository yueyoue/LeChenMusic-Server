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
	_, err := tx.ExecContext(ctx, `ALTER TABLE library ADD COLUMN media_type VARCHAR(20) DEFAULT 'music'`)
	return err
}

func downAddLibraryMediaType(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't support DROP COLUMN directly
	return nil
}
