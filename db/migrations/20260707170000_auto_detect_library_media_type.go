// [LeChenMusic] Auto-detect library media type based on name/path
package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAutoDetectLibraryMediaType, downAutoDetectLibraryMediaType)
}

func upAutoDetectLibraryMediaType(ctx context.Context, tx *sql.Tx) error {
	// Update libraries that look like audiobook libraries based on name/path keywords
	// Use INSTR instead of LIKE for better Unicode support
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

func downAutoDetectLibraryMediaType(_ context.Context, _ *sql.Tx) error {
	return nil
}
