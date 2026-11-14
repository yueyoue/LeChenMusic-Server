package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddDownloadTask, downAddDownloadTask)
}

func upAddDownloadTask(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:online-download]
	_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS download_task (
	id                VARCHAR(255)  NOT NULL PRIMARY KEY,
	user_id           VARCHAR(255)  NOT NULL DEFAULT '',
	source            VARCHAR(64)   NOT NULL DEFAULT '',
	platform          VARCHAR(64)   NOT NULL DEFAULT '',
	song_id           VARCHAR(255)  NOT NULL DEFAULT '',
	song_name         VARCHAR(512)  NOT NULL DEFAULT '',
	artist            VARCHAR(512)  NOT NULL DEFAULT '',
	quality           VARCHAR(32)   NOT NULL DEFAULT '',
	actual_format     VARCHAR(16)   NOT NULL DEFAULT '',
	actual_bitrate    INTEGER       NOT NULL DEFAULT 0,
	status            VARCHAR(32)   NOT NULL DEFAULT 'queued',
	progress_bytes    INTEGER       NOT NULL DEFAULT 0,
	total_bytes       INTEGER       NOT NULL DEFAULT 0,
	file_path         VARCHAR(1024) NOT NULL DEFAULT '',
	error             VARCHAR(1024) NOT NULL DEFAULT '',
	upstream_time     INTEGER       NOT NULL DEFAULT 0,
	sign              VARCHAR(255)  NOT NULL DEFAULT '',
	created_at        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at       TIMESTAMP
);

CREATE INDEX IF NOT EXISTS download_task_status_idx ON download_task(status);
CREATE INDEX IF NOT EXISTS download_task_created_at_idx ON download_task(created_at);
`)
	// [LeChenMusic-END:online-download]
	return err
}

func downAddDownloadTask(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS download_task;`)
	return err
}
