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
	// Update libraries with empty or 'music' media_type that look like audiobook libraries
	// based on their name containing audiobook-related keywords
	keywords := []string{"有声", "audiobook", "评书", "相声", "小说", "戏曲"}
	for _, kw := range keywords {
		_, err := tx.ExecContext(ctx,
			`UPDATE library SET media_type = 'audiobook' WHERE (media_type = '' OR media_type IS NULL OR media_type = 'music') AND (name LIKE '%' || ? || '%' OR path LIKE '%' || ? || '%')`,
			kw, kw)
		if err != nil {
			return err
		}
	}

	// Ensure all libraries have a media_type (default to 'music')
	_, err := tx.ExecContext(ctx,
		`UPDATE library SET media_type = 'music' WHERE media_type = '' OR media_type IS NULL`)
	return err
}

func downAutoDetectLibraryMediaType(_ context.Context, _ *sql.Tx) error {
	return nil
}
