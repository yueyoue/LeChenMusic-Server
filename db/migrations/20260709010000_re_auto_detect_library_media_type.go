// [LeChenMusic] Re-run auto-detection for library media types
// This migration ensures libraries with audiobook-related names/paths
// have their media_type set correctly, even if they were created after
// the initial auto-detect migration ran.
package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upReAutoDetectLibraryMediaType, downReAutoDetectLibraryMediaType)
}

func upReAutoDetectLibraryMediaType(ctx context.Context, tx *sql.Tx) error {
	// Re-run auto-detection for libraries that still have empty or 'music' media_type
	// but whose name or path suggests they should be audiobook libraries.
	keywords := []string{"有声", "audiobook", "评书", "相声", "小说", "戏曲"}
	for _, kw := range keywords {
		_, err := tx.ExecContext(ctx,
			`UPDATE library SET media_type = 'audiobook' WHERE (media_type = '' OR media_type IS NULL OR media_type = 'music') AND (INSTR(name, ?) > 0 OR INSTR(path, ?) > 0)`,
			kw, kw)
		if err != nil {
			return err
		}
	}

	// Also check path for common audiobook directory names
	_, err := tx.ExecContext(ctx,
		`UPDATE library SET media_type = 'audiobook' WHERE (media_type = '' OR media_type IS NULL OR media_type = 'music') AND (INSTR(path, 'audiobook') > 0 OR INSTR(path, 'Audiobook') > 0)`)
	if err != nil {
		return err
	}

	// Ensure all libraries have a media_type (default to 'music')
	_, err = tx.ExecContext(ctx,
		`UPDATE library SET media_type = 'music' WHERE media_type = '' OR media_type IS NULL`)
	return err
}

func downReAutoDetectLibraryMediaType(_ context.Context, _ *sql.Tx) error {
	return nil
}
