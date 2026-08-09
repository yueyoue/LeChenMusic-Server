package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/db"
	"github.com/navidrome/navidrome/log"
	"github.com/spf13/cobra"
)

var (
	backupCount    int
	backupDir      string
	force          bool
	restorePath    string
	includeCache   bool
)

func init() {
	rootCmd.AddCommand(backupRoot)

	backupCmd.Flags().StringVarP(&backupDir, "backup-dir", "d", "", "directory to manually make backup")
	backupCmd.Flags().BoolVar(&includeCache, "cache", true, "include artwork cache in backup")
	backupRoot.AddCommand(backupCmd)

	pruneCmd.Flags().StringVarP(&backupDir, "backup-dir", "d", "", "directory holding Navidrome backups")
	pruneCmd.Flags().IntVarP(&backupCount, "keep-count", "k", -1, "specify the number of backups to keep. 0 remove ALL backups, and negative values mean to use the default from configuration")
	pruneCmd.Flags().BoolVarP(&force, "force", "f", false, "bypass warning when backup count is zero")
	backupRoot.AddCommand(pruneCmd)

	restoreCommand.Flags().StringVarP(&restorePath, "backup-file", "b", "", "path of backup database or archive to restore")
	restoreCommand.Flags().BoolVarP(&force, "force", "f", false, "bypass restore warning")
	_ = restoreCommand.MarkFlagRequired("backup-file")
	backupRoot.AddCommand(restoreCommand)
}

var (
	backupRoot = &cobra.Command{
		Use:     "backup",
		Aliases: []string{"bkp"},
		Short:   "Create, restore and prune database backups",
		Long:    "Create, restore and prune database backups. Backups include the database and optionally the artwork cache.",
	}

	backupCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a backup",
		Long:  "Create a backup of the database and optionally the artwork cache. This will ignore BackupCount",
		Run: func(cmd *cobra.Command, _ []string) {
			runBackup(cmd.Context())
		},
	}

	pruneCmd = &cobra.Command{
		Use:   "prune",
		Short: "Prune backups",
		Long:  "Manually prune backups according to backup rules",
		Run: func(cmd *cobra.Command, _ []string) {
			runPrune(cmd.Context())
		},
	}

	restoreCommand = &cobra.Command{
		Use:   "restore",
		Short: "Restore from backup",
		Long:  "Restore database and cache from a backup. This must be done offline (server stopped).",
		Run: func(cmd *cobra.Command, _ []string) {
			runRestore(cmd.Context())
		},
	}
)

func getBackupDir() string {
	if backupDir != "" {
		return backupDir
	}
	if path, err := conf.Server.Backup.Path.Path(); err == nil {
		return path
	}
	return filepath.Join(conf.Server.DataFolder, "backups")
}

func getDbPath() string {
	idx := strings.LastIndex(conf.Server.DbPath, "?")
	if idx == -1 {
		return conf.Server.DbPath
	}
	return conf.Server.DbPath[:idx]
}

func getCacheDir() string {
	return filepath.Join(conf.Server.DataFolder, "cache")
}

func backupFileName(t time.Time, ext string) string {
	return filepath.Join(
		getBackupDir(),
		fmt.Sprintf("navidrome_backup_%s%s", t.Format("2006.01.02_15.04.05"), ext),
	)
}

func runBackup(ctx context.Context) {
	dbPath := getDbPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatal("No existing database", "path", dbPath)
		return
	}

	// Ensure backup directory exists
	backupPath := getBackupDir()
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		log.Fatal("Cannot create backup directory", "path", backupPath, err)
		return
	}

	start := time.Now()

	if includeCache {
		// Create tar.gz archive with database + cache
		archivePath := backupFileName(start, ".tar.gz")
		if err := createBackupArchive(ctx, archivePath, dbPath, getCacheDir()); err != nil {
			log.Fatal("Error creating backup archive", err)
		}
		elapsed := time.Since(start)
		log.Info("Backup complete (database + cache)", "elapsed", elapsed, "path", archivePath)
	} else {
		// Database only (original behavior)
		path, err := db.Backup(ctx)
		if err != nil {
			log.Fatal("Error backing up database", "backup path", conf.Server.BasePath, err)
		}
		elapsed := time.Since(start)
		log.Info("Backup complete (database only)", "elapsed", elapsed, "path", path)
	}
}

func createBackupArchive(ctx context.Context, archivePath, dbPath, cacheDir string) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("creating archive file: %w", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add database file
	if err := addFileToTar(tw, dbPath, "navidrome.db"); err != nil {
		return fmt.Errorf("adding database to archive: %w", err)
	}
	log.Debug(ctx, "Added database to backup", "path", dbPath)

	// Add cache directory
	if _, err := os.Stat(cacheDir); err == nil {
		err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(filepath.Dir(cacheDir), path)
			if info.IsDir() {
				return nil
			}
			return addFileToTar(tw, path, "cache/"+relPath)
		})
		if err != nil {
			return fmt.Errorf("adding cache to archive: %w", err)
		}
		log.Debug(ctx, "Added cache to backup", "path", cacheDir)
	} else {
		log.Warn(ctx, "Cache directory not found, skipping", "path", cacheDir)
	}

	return nil
}

func addFileToTar(tw *tar.Writer, filePath, nameInArchive string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = nameInArchive

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}

func runPrune(ctx context.Context) {
	if backupCount != -1 {
		conf.Server.Backup.Count = backupCount
	}

	if conf.Server.Backup.Count == 0 && !force {
		fmt.Println("Warning: pruning ALL backups")
		fmt.Printf("Please enter YES (all caps) to continue: ")
		var input string
		_, err := fmt.Scanln(&input)

		if input != "YES" || err != nil {
			log.Warn("Prune cancelled")
			return
		}
	}

	dbPath := getDbPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatal("No existing database", "path", dbPath)
		return
	}

	start := time.Now()
	count, err := db.Prune(ctx)
	if err != nil {
		log.Fatal("Error pruning database", "backup path", conf.Server.BasePath, err)
	}

	// Also prune old archive files
	archiveCount := pruneArchives(ctx)

	elapsed := time.Since(start)
	log.Info("Prune complete", "elapsed", elapsed, "dbBackupsPruned", count, "archivesPruned", archiveCount)
}

func pruneArchives(ctx context.Context) int {
	backupPath := getBackupDir()
	files, err := os.ReadDir(backupPath)
	if err != nil {
		return 0
	}

	var archives []os.DirEntry
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".tar.gz") && strings.HasPrefix(f.Name(), "navidrome_backup_") {
			archives = append(archives, f)
		}
	}

	if len(archives) <= conf.Server.Backup.Count {
		return 0
	}

	// Sort by name (which includes timestamp) and delete oldest
	// Simple bubble sort for small lists
	for i := range archives {
		for j := i + 1; j < len(archives); j++ {
			if archives[i].Name() < archives[j].Name() {
				archives[i], archives[j] = archives[j], archives[i]
			}
		}
	}

	deleted := 0
	for _, f := range archives[conf.Server.Backup.Count:] {
		path := filepath.Join(backupPath, f.Name())
		if err := os.Remove(path); err != nil {
			log.Error(ctx, "Failed to delete archive", "path", path, err)
		} else {
			deleted++
		}
	}
	return deleted
}

func runRestore(ctx context.Context) {
	// Check if the file is an archive or a plain database
	isArchive := strings.HasSuffix(restorePath, ".tar.gz")

	if isArchive {
		runRestoreArchive(ctx)
	} else {
		runRestoreDB(ctx)
	}
}

func runRestoreDB(ctx context.Context) {
	dbPath := getDbPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatal("No existing database", "path", dbPath)
		return
	}

	if !force {
		fmt.Println("Warning: restoring the Navidrome database should only be done offline.")
		fmt.Println("NOTE: This only restores the database. Artwork cache is NOT included.")
		fmt.Println("      After restore, run a full scan to rebuild artwork from audio files.")
		fmt.Printf("Please enter YES (all caps) to continue: ")
		var input string
		_, err := fmt.Scanln(&input)

		if input != "YES" || err != nil {
			log.Warn("Restore cancelled")
			return
		}
	}

	start := time.Now()
	err := db.Restore(ctx, restorePath)
	if err != nil {
		log.Fatal("Error restoring database", "backup path", conf.Server.BasePath, err)
	}

	elapsed := time.Since(start)
	log.Info("Restore complete", "elapsed", elapsed)
	fmt.Println("\nIMPORTANT: Run a full scan after starting the server to rebuild artwork cache.")
}

func runRestoreArchive(ctx context.Context) {
	if !force {
		fmt.Println("Warning: restoring from archive will overwrite the current database and cache.")
		fmt.Printf("Please enter YES (all caps) to continue: ")
		var input string
		_, err := fmt.Scanln(&input)

		if input != "YES" || err != nil {
			log.Warn("Restore cancelled")
			return
		}
	}

	start := time.Now()

	file, err := os.Open(restorePath)
	if err != nil {
		log.Fatal("Error opening archive", "path", restorePath, err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		log.Fatal("Error reading gzip", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	dataDir := conf.Server.DataFolder
	dbPath := getDbPath()
	cacheDir := getCacheDir()

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Error reading tar", err)
		}

		var destPath string
		switch {
		case header.Name == "navidrome.db":
			destPath = dbPath
		case strings.HasPrefix(header.Name, "cache/"):
			relPath := strings.TrimPrefix(header.Name, "cache/")
			destPath = filepath.Join(cacheDir, relPath)
		default:
			log.Warn(ctx, "Skipping unknown entry in archive", "name", header.Name)
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			log.Fatal("Cannot create directory", "path", filepath.Dir(destPath), err)
		}

		// Write file
		outFile, err := os.Create(destPath)
		if err != nil {
			log.Fatal("Cannot create file", "path", destPath, err)
		}

		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			log.Fatal("Error writing file", "path", destPath, err)
		}
		outFile.Close()

		log.Debug(ctx, "Restored", "path", destPath)
	}

	elapsed := time.Since(start)
	log.Info("Restore complete (database + cache)", "elapsed", elapsed)
	fmt.Printf("Restored to: %s\n", dataDir)
}
