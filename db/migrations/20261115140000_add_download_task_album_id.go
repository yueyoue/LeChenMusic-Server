package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddDownloadTaskAlbumID, downAddDownloadTaskAlbumID)
}

func upAddDownloadTaskAlbumID(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:online-download]
	// Album id unlocks the album-level scrape fields (album artist + release
	// year) that song-level search results do not carry.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE download_task ADD COLUMN album_id VARCHAR(64) NOT NULL DEFAULT '';`); err != nil {
		return err
	}
	// [LeChenMusic-END:online-download]
	return nil
}

func downAddDownloadTaskAlbumID(ctx context.Context, tx *sql.Tx) error {
	// SQLite cannot drop columns on old versions; the table is dropped by the
	// original download_task migration's down() on a full rollback.
	return nil
}