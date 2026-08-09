package cmd

import (
	"context"
	"fmt"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/spf13/cobra"
)

var (
	dedupeDryRun  bool
	dedupeLibrary string
)

func init() {
	dedupeCmd.Flags().BoolVarP(&dedupeDryRun, "dry-run", "n", false, "only show duplicates, don't delete")
	dedupeCmd.Flags().StringVarP(&dedupeLibrary, "library", "l", "", "library name or ID to dedupe (default: all)")
	rootCmd.AddCommand(dedupeCmd)
}

var dedupeCmd = &cobra.Command{
	Use:   "dedupe",
	Short: "Remove duplicate media files from the database",
	Long: `Remove duplicate media files that may have been created during backup/restore.

Duplicates are detected by file path. When multiple entries exist for the same path,
the oldest entry (earliest created_at) is kept and newer duplicates are deleted.

This command should be run offline (with the server stopped).`,
	Run: func(cmd *cobra.Command, _ []string) {
		runDedupe(cmd.Context())
	},
}

func runDedupe(ctx context.Context) {
	ds, ctx := getAdminContext(ctx)

	log.Info(ctx, "Starting deduplication", "dryRun", dedupeDryRun)

	// Get all media files
	mfRepo := ds.MediaFile(ctx)
	cursor, err := mfRepo.GetCursor(model.QueryOptions{})
	if err != nil {
		log.Fatal(ctx, "Error loading media files", err)
	}

	// Group by path
	type pathEntry struct {
		keep   model.MediaFile
		dupes  []model.MediaFile
	}
	paths := make(map[string]*pathEntry)
	total := 0

	for mf, err := range cursor {
		if err != nil {
			log.Fatal(ctx, "Error reading media file", err)
		}
		total++

		entry, exists := paths[mf.Path]
		if !exists {
			paths[mf.Path] = &pathEntry{keep: mf}
		} else {
			// Keep the oldest entry (earliest created_at)
			if mf.CreatedAt.Before(entry.keep.CreatedAt) {
				entry.dupes = append(entry.dupes, entry.keep)
				entry.keep = mf
			} else {
				entry.dupes = append(entry.dupes, mf)
			}
		}
	}

	// Count duplicates
	var allDupes []model.MediaFile
	for _, entry := range paths {
		allDupes = append(allDupes, entry.dupes...)
	}

	fmt.Printf("Total media files: %d\n", total)
	fmt.Printf("Unique paths: %d\n", len(paths))
	fmt.Printf("Duplicates found: %d\n", len(allDupes))

	if len(allDupes) == 0 {
		fmt.Println("No duplicates found. Nothing to do.")
		return
	}

	if dedupeDryRun {
		fmt.Println("\n--- Duplicates (dry-run, not deleting) ---")
		for _, dupe := range allDupes {
			fmt.Printf("  DELETE: %s (ID: %s, Created: %s)\n", dupe.Path, dupe.ID, dupe.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("\nRun without --dry-run to actually delete %d duplicates.\n", len(allDupes))
		return
	}

	// Delete duplicates
	deleted := 0
	for _, dupe := range allDupes {
		if err := mfRepo.Delete(dupe.ID); err != nil {
			log.Error(ctx, "Error deleting duplicate", "id", dupe.ID, "path", dupe.Path, err)
		} else {
			deleted++
		}
	}

	fmt.Printf("\nDeleted %d duplicate media files.\n", deleted)
	log.Info(ctx, "Deduplication complete", "deleted", deleted)
}
