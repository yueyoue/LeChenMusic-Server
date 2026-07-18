package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddAudiobookCoverUrl, downAddAudiobookCoverUrl)
}

func upAddAudiobookCoverUrl(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:audiobook]
	_, err := tx.ExecContext(ctx, `ALTER TABLE audiobook ADD COLUMN cover_url VARCHAR(2048) DEFAULT ''`)
	if err != nil {
		// Column might already exist, ignore error
		return nil
	}
	// [LeChenMusic-END:audiobook]
	return nil
}

func downAddAudiobookCoverUrl(ctx context.Context, tx *sql.Tx) error {
	// SQLite doesn't support DROP COLUMN in older versions
	return nil
}
