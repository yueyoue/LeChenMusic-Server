package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddOnlineOpLog, downAddOnlineOpLog)
}

func upAddOnlineOpLog(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:online-download]
	_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS online_op_log (
	id         VARCHAR(255)  NOT NULL PRIMARY KEY,
	user_id    VARCHAR(255)  NOT NULL DEFAULT '',
	user_name  VARCHAR(255)  NOT NULL DEFAULT '',
	action     VARCHAR(64)   NOT NULL DEFAULT '',
	song_name  VARCHAR(512)  NOT NULL DEFAULT '',
	artist     VARCHAR(512)  NOT NULL DEFAULT '',
	detail     VARCHAR(2048) NOT NULL DEFAULT '',
	success    BOOLEAN       NOT NULL DEFAULT TRUE,
	created_at TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS online_op_log_created_at_idx ON online_op_log(created_at);
CREATE INDEX IF NOT EXISTS online_op_log_action_idx ON online_op_log(action);
`)
	// [LeChenMusic-END:online-download]
	return err
}

func downAddOnlineOpLog(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS online_op_log;`)
	return err
}
