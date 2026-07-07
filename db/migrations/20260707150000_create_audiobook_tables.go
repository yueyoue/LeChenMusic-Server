package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upCreateAudiobookTables, downCreateAudiobookTables)
}

func upCreateAudiobookTables(ctx context.Context, tx *sql.Tx) error {
	// [LeChenMusic-START:audiobook]
	_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS audiobook (
    id             VARCHAR(255) PRIMARY KEY,
    title          VARCHAR(255) NOT NULL,
    author         VARCHAR(255) DEFAULT '',
    narrator       VARCHAR(255) DEFAULT '',
    description    TEXT DEFAULT '',
    genre          VARCHAR(128) DEFAULT '有声读物',
    year           INTEGER DEFAULT 0,
    cover_path     VARCHAR(1024) DEFAULT '',
    total_duration INTEGER DEFAULT 0,
    chapter_count  INTEGER DEFAULT 0,
    series         VARCHAR(255) DEFAULT '',
    library_id     INTEGER REFERENCES library(id),
    path           VARCHAR(1024) NOT NULL,
    hash           VARCHAR(64) DEFAULT '',
    size           INTEGER DEFAULT 0,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audiobook_chapter (
    id             VARCHAR(255) PRIMARY KEY,
    audiobook_id   VARCHAR(255) REFERENCES audiobook(id) ON DELETE CASCADE,
    title          VARCHAR(255) NOT NULL,
    chapter_number INTEGER NOT NULL,
    duration       INTEGER DEFAULT 0,
    format         VARCHAR(20) DEFAULT '',
    file_size      INTEGER DEFAULT 0,
    path           VARCHAR(1024) NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audiobook_progress (
    id              VARCHAR(255) PRIMARY KEY,
    user_id         VARCHAR(255) REFERENCES user(id) ON DELETE CASCADE,
    audiobook_id    VARCHAR(255) REFERENCES audiobook(id) ON DELETE CASCADE,
    chapter_id      VARCHAR(255) REFERENCES audiobook_chapter(id),
    chapter_number  INTEGER DEFAULT 0,
    position        INTEGER DEFAULT 0,
    playback_speed  REAL DEFAULT 1.0,
    skip_intro      INTEGER DEFAULT 0,
    skip_outro      INTEGER DEFAULT 0,
    completed       BOOLEAN DEFAULT FALSE,
    last_played_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, audiobook_id)
);

CREATE TABLE IF NOT EXISTS audiobook_bookmark (
    id            VARCHAR(255) PRIMARY KEY,
    user_id       VARCHAR(255) REFERENCES user(id) ON DELETE CASCADE,
    audiobook_id  VARCHAR(255) REFERENCES audiobook(id) ON DELETE CASCADE,
    chapter_id    VARCHAR(255) REFERENCES audiobook_chapter(id),
    position      INTEGER NOT NULL,
    title         VARCHAR(255) DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audiobook_favorite (
    id            VARCHAR(255) PRIMARY KEY,
    user_id       VARCHAR(255) REFERENCES user(id) ON DELETE CASCADE,
    audiobook_id  VARCHAR(255) REFERENCES audiobook(id) ON DELETE CASCADE,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, audiobook_id)
);

CREATE INDEX IF NOT EXISTS audiobook_chapter_audiobook_id ON audiobook_chapter(audiobook_id);
CREATE INDEX IF NOT EXISTS audiobook_chapter_number ON audiobook_chapter(audiobook_id, chapter_number);
CREATE INDEX IF NOT EXISTS audiobook_progress_user ON audiobook_progress(user_id);
CREATE INDEX IF NOT EXISTS audiobook_progress_book ON audiobook_progress(audiobook_id);
CREATE INDEX IF NOT EXISTS audiobook_bookmark_user ON audiobook_bookmark(user_id, audiobook_id);
CREATE INDEX IF NOT EXISTS audiobook_favorite_user ON audiobook_favorite(user_id);
CREATE INDEX IF NOT EXISTS audiobook_genre ON audiobook(genre);
CREATE INDEX IF NOT EXISTS audiobook_narrator ON audiobook(narrator);
CREATE INDEX IF NOT EXISTS audiobook_library ON audiobook(library_id);
`)
	if err != nil {
		return err
	}
	// [LeChenMusic-END:audiobook]
	return nil
}

func downCreateAudiobookTables(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
DROP TABLE IF EXISTS audiobook_favorite;
DROP TABLE IF EXISTS audiobook_bookmark;
DROP TABLE IF EXISTS audiobook_progress;
DROP TABLE IF EXISTS audiobook_chapter;
DROP TABLE IF EXISTS audiobook;
`)
	return err
}
