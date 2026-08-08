package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddUserStats, downAddUserStats)
}

func upAddUserStats(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:user-stats]
	_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS play_log (
	id           VARCHAR(255) NOT NULL PRIMARY KEY,
	user_id      VARCHAR(255) NOT NULL,
	item_type    VARCHAR(32)  NOT NULL DEFAULT '',
	item_id      VARCHAR(255) NOT NULL DEFAULT '',
	item_title   VARCHAR(512) NOT NULL DEFAULT '',
	item_artist  VARCHAR(512) NOT NULL DEFAULT '',
	duration     INTEGER      NOT NULL DEFAULT 0,
	play_date    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS play_log_user_id_idx ON play_log(user_id);
CREATE INDEX IF NOT EXISTS play_log_play_date_idx ON play_log(play_date);

CREATE TABLE IF NOT EXISTS user_device (
	id           VARCHAR(255) NOT NULL PRIMARY KEY,
	user_id      VARCHAR(255) NOT NULL,
	device_id    VARCHAR(255) NOT NULL,
	device_info  VARCHAR(1024) NOT NULL DEFAULT '',
	app_version  VARCHAR(64)  NOT NULL DEFAULT '',
	last_seen_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS user_device_user_device_idx ON user_device(user_id, device_id);
CREATE INDEX IF NOT EXISTS user_device_user_id_idx ON user_device(user_id);
`)
	// [LeChenMusic-END:user-stats]
	return err
}

func downAddUserStats(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
DROP TABLE IF EXISTS user_device;
DROP TABLE IF EXISTS play_log;
`)
	return err
}
