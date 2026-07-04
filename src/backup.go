// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// RunBackup creates backups of all linked files
func (app *App) RunBackup(configs []Config) error {
	app.logger.heading("Creating backups...")

	if err := os.MkdirAll(app.backupDir, 0755); err != nil {
		return fmt.Errorf("error creating backup directory: %w", err)
	}

	for _, config := range configs {
		for target := range config.Link {
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

			backupPath := app.getBackupPath(targetPath)
			app.logger.info("Backing up: %s → %s", targetPath, backupPath)

			if err := app.logger.execute(func() error {
				if isDir {
					return copyDir(targetPath, backupPath)
				}
				return copyFile(targetPath, backupPath)
			}); err != nil {
				app.logger.error("Error creating backup: %v", err)
			} else {
				app.logger.success("Backed up: %s", targetPath)
			}
		}
	}

	app.logger.summary()
	return nil
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

	if len(entries) == 0 {
		fmt.Println("No backups found.")
		return nil
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fmt.Printf("  %s  %s\n", info.ModTime().Format("2006-01-02 15:04:05"), entry.Name())
	}

	return nil
}

func (app *App) createBackup(targetPath string, isDir bool) {
	backupPath := app.getBackupPath(targetPath)

	app.logger.info("Creating backup: %s → %s", targetPath, backupPath)
	if err := app.logger.execute(func() error {
		// Ensure backup directory exists
		if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
			return err
		}
		if isDir {
			return copyDir(targetPath, backupPath)
		}
		return copyFile(targetPath, backupPath)
	}); err != nil {
		app.logger.error("Error creating backup: %v", err)
	}
}

func (app *App) restoreBackup(targetPath string) {
	backupPath := app.getBackupPath(targetPath)

	exists, isDir, _ := checkPathExists(backupPath)
	if !exists {
		app.logger.debug("No backup found for: %s", targetPath)
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
