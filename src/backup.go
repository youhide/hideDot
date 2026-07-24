// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// manifestName is the index kept alongside the backups. Backup directories are
// named after a hash of their original path, which makes them unreadable on
// their own — the manifest is what lets `backup list` show real paths.
const manifestName = "manifest.json"

// backupEntry describes one backup in the manifest.
type backupEntry struct {
	OriginalPath string `json:"original_path"`
	Timestamp    string `json:"timestamp"`
	IsDir        bool   `json:"is_dir"`
}

// RunBackup creates backups of all linked files
func (app *App) RunBackup(configs []Config) error {
	app.logger.heading("Creating backups...")

	if err := os.MkdirAll(app.backupDir, 0755); err != nil {
		return fmt.Errorf("error creating backup directory: %w", err)
	}

	for _, config := range configs {
		for _, target := range slices.Sorted(maps.Keys(config.Link)) {
			targetPath := expandPath(target, app.homeDir)

			exists, isDir, _ := checkPathExists(targetPath)
			if !exists {
				continue
			}

			info, err := os.Lstat(targetPath)
			if err != nil {
				continue
			}

			// Skip if it's already a symlink
			if info.Mode()&os.ModeSymlink != 0 {
				app.logger.debug("Skipping symlink: %s", targetPath)
				continue
			}

			if err := app.createBackup(targetPath, isDir); err != nil {
				app.logger.error("Error creating backup: %v", err)
				continue
			}
			app.logger.success("Backed up: %s", targetPath)
		}
	}

	app.logger.summary()
	return app.failureError()
}

// RunListBackups lists all available backups
func (app *App) RunListBackups() error {
	app.logger.heading("Available backups in %s", app.backupDir)

	entries, err := os.ReadDir(app.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found.")
			return nil
		}
		return fmt.Errorf("error reading backup directory: %w", err)
	}

	manifest := app.readManifest()
	var count int

	for _, entry := range entries {
		if entry.Name() == manifestName {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		count++

		when := info.ModTime().Format("2006-01-02 15:04:05")
		name := entry.Name()

		// Fall back to the raw directory name for backups made before the
		// manifest existed, or if it was removed by hand.
		if record, ok := manifest[entry.Name()]; ok {
			name = record.OriginalPath
			if t, err := time.Parse(time.RFC3339, record.Timestamp); err == nil {
				when = t.Format("2006-01-02 15:04:05")
			}
		}

		fmt.Printf("  %s  %s\n", when, name)
	}

	if count == 0 {
		fmt.Println("No backups found.")
	}

	return nil
}

// createBackup copies targetPath into the backup directory, replacing any
// previous backup of the same path. It returns an error instead of only logging
// one so callers can refuse to destroy a file they failed to back up.
func (app *App) createBackup(targetPath string, isDir bool) error {
	backupPath := app.getBackupPath(targetPath)

	app.logger.info("Creating backup: %s → %s", targetPath, backupPath)
	return app.logger.execute(func() error {
		if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
			return err
		}

		// Drop the previous backup first: copyDir merges into an existing
		// directory, which would leave files that were since deleted behind.
		if err := os.RemoveAll(backupPath); err != nil {
			return err
		}

		if isDir {
			if err := copyDir(targetPath, backupPath); err != nil {
				return err
			}
		} else if err := copyFile(targetPath, backupPath); err != nil {
			return err
		}

		return app.recordBackup(targetPath, isDir)
	})
}

func (app *App) restoreBackup(targetPath string) {
	backupPath := app.getBackupPath(targetPath)

	exists, isDir, _ := checkPathExists(backupPath)
	if !exists {
		app.logger.warn("No backup to restore for: %s", targetPath)
		return
	}

	app.logger.info("Restoring backup: %s → %s", backupPath, targetPath)
	if err := app.logger.execute(func() error {
		if isDir {
			return copyDir(backupPath, targetPath)
		}
		return copyFile(backupPath, targetPath)
	}); err != nil {
		app.logger.error("Error restoring backup: %v", err)
	} else {
		app.logger.success("Restored: %s", targetPath)
	}
}

func (app *App) getBackupPath(targetPath string) string {
	// Create a hash of the path for unique identification
	hash := sha256.Sum256([]byte(targetPath))
	shortHash := hex.EncodeToString(hash[:])[:8]

	baseName := filepath.Base(targetPath)
	return filepath.Join(app.backupDir, fmt.Sprintf("%s_%s", baseName, shortHash))
}

func (app *App) manifestPath() string {
	return filepath.Join(app.backupDir, manifestName)
}

// readManifest returns the backup index, or an empty map when it is missing or
// unreadable — listing backups must never fail because of a broken index.
func (app *App) readManifest() map[string]backupEntry {
	manifest := make(map[string]backupEntry)

	data, err := os.ReadFile(app.manifestPath())
	if err != nil {
		return manifest
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		app.logger.debug("Ignoring unreadable backup manifest: %v", err)
		return make(map[string]backupEntry)
	}

	return manifest
}

// recordBackup adds targetPath to the manifest so it can be listed by its real
// name later.
func (app *App) recordBackup(targetPath string, isDir bool) error {
	manifest := app.readManifest()
	manifest[filepath.Base(app.getBackupPath(targetPath))] = backupEntry{
		OriginalPath: targetPath,
		Timestamp:    time.Now().Format(time.RFC3339),
		IsDir:        isDir,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return writeFileAtomic(app.manifestPath(), data)
}
