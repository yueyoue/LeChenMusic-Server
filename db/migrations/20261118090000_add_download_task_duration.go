package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddDownloadTaskDuration, downAddDownloadTaskDuration)
}

func upAddDownloadTaskDuration(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:online-download]
	// Expected song length (seconds) from the upstream search result: the
	// download pipeline verifies the real audio duration against it so that
	// upstream ad/trial placeholder audio never reaches the library.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE download_task ADD COLUMN duration INTEGER NOT NULL DEFAULT 0;`); err != nil {
		return err
	}
	// [LeChenMusic-END:online-download]
	return nil
}

func downAddDownloadTaskDuration(ctx context.Context, tx *sql.Tx) error {
	// SQLite cannot drop columns on old versions; the table is dropped by the
	// original download_task migration's down() on a full rollback.
	return nil
}
