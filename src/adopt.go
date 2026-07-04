// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunAdopt moves an existing file/dir into the dotfiles directory and replaces
// it with a symlink pointing back to the moved copy.
func (app *App) RunAdopt(target string) error {
	targetPath := expandPath(target, app.homeDir)
	targetPath, _ = filepath.Abs(targetPath)

	info, err := os.Lstat(targetPath)
	if err != nil {
		return fmt.Errorf("cannot adopt %s: %w", targetPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is already a symlink, nothing to adopt", targetPath)
	}
	isDir := info.IsDir()

	dest := filepath.Join(app.execDir, filepath.Base(targetPath))
	if destExists, _, _ := checkPathExists(dest); destExists {
		return fmt.Errorf("destination already exists: %s", dest)
	}

	if !app.noBackup {
		app.createBackup(targetPath, isDir)
	}

	app.logger.info("Moving %s → %s", targetPath, dest)
	if err := app.logger.execute(func() error {
		if isDir {
			if err := copyDir(targetPath, dest); err != nil {
				return err
			}
		} else if err := copyFile(targetPath, dest); err != nil {
			return err
		}
		return os.RemoveAll(targetPath)
	}); err != nil {
		return fmt.Errorf("error moving path: %w", err)
	}

	app.logger.info("Creating symlink: %s → %s", targetPath, dest)
	if err := app.logger.execute(func() error {
		return os.Symlink(dest, targetPath)
	}); err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}

	if !app.dryRun {
		app.logger.success("Adopted: %s", targetPath)
	}

	// Suggest the config entry, using ~ and a repo-relative source when possible.
	linkTarget := targetPath
	if rel, err := filepath.Rel(app.homeDir, targetPath); err == nil && !strings.HasPrefix(rel, "..") {
		linkTarget = "~/" + rel
	}
	linkSource := "./" + filepath.Base(dest)
	if rel, err := filepath.Rel(app.execDir, dest); err == nil && !strings.HasPrefix(rel, "..") {
		linkSource = "./" + rel
	}
	app.logger.heading("Add this to your config:")
	fmt.Printf("  link:\n    %s: %s\n", linkTarget, linkSource)
	return nil
}
