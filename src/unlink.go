// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "os"

// RunUnlink removes symlinks and optionally restores backups
func (app *App) RunUnlink(configs []Config, restore bool) error {
	for _, config := range configs {
		if len(config.Link) > 0 {
			app.logger.heading("Removing symlinks...")
			for target := range config.Link {
				targetPath := expandPath(target, app.homeDir)

				// Check if target exists and is a symlink
				info, err := os.Lstat(targetPath)
				if err != nil {
					if os.IsNotExist(err) {
						app.logger.info("Symlink does not exist: %s", targetPath)
						continue
					}
					app.logger.error("Error checking %s: %v", targetPath, err)
					continue
				}

				if info.Mode()&os.ModeSymlink == 0 {
					app.logger.warn("Not a symlink, skipping: %s", targetPath)
					continue
				}

				app.logger.info("Removing symlink: %s", targetPath)
				if err := app.logger.execute(func() error {
					return os.Remove(targetPath)
				}); err != nil {
					app.logger.error("Error removing symlink: %v", err)
					continue
				}
				app.logger.success("Removed: %s", targetPath)

				// Restore backup if requested
				if restore {
					app.restoreBackup(targetPath)
				}
			}
		}
	}

	app.logger.summary()
	return nil
}
