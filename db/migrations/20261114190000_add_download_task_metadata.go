package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddDownloadTaskMetadata, downAddDownloadTaskMetadata)
}

func upAddDownloadTaskMetadata(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:online-download]
	if _, err := tx.ExecContext(ctx, `ALTER TABLE download_task ADD COLUMN album_name VARCHAR(512) NOT NULL DEFAULT '';`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE download_task ADD COLUMN cover_url VARCHAR(1024) NOT NULL DEFAULT '';`); err != nil {
		return err
	}
	// [LeChenMusic-END:online-download]
	return nil
}

func downAddDownloadTaskMetadata(ctx context.Context, tx *sql.Tx) error {
	// SQLite cannot drop columns on old versions; table is dropped by the
	// previous migration's down() if a full rollback is needed.
	return nil
}
