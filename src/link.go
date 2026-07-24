// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// RunLink executes the link command
func (app *App) RunLink(configs []Config) error {
	declared := app.declaredTargets(configs)

	for _, config := range configs {
		opts := app.getDefaultOptions(config)

		if config.Defaults != nil {
			app.logger.info("Settings: force=%v, relink=%v, backup=%v", opts.force, opts.relink, opts.backup)
		}

		// Run pre-link hooks. A failing pre-hook means the section's
		// preconditions aren't met, so skip the whole section rather than
		// linking on top of a half-prepared system.
		if config.Hooks != nil && len(config.Hooks.PreLink) > 0 {
			app.logger.heading("Running pre-link hooks...")
			if err := app.runHooks(config.Hooks.PreLink); err != nil {
				app.logger.error("Pre-link hook failed, skipping this config section: %v", err)
				continue
			}
		}

		// Process directory creation
		if len(config.Create) > 0 {
			app.logger.heading("Creating directories...")
			for _, dir := range config.Create {
				app.createDirectory(dir)
			}
		}

		// Process link creation. Maps iterate in random order, so sort the
		// keys to keep runs (and their output) reproducible.
		if len(config.Link) > 0 {
			app.logger.heading("Creating links...")
			for _, target := range slices.Sorted(maps.Keys(config.Link)) {
				app.createLink(target, config.Link[target], opts, declared)
			}
		}

		// Run post-link hooks
		if config.Hooks != nil && len(config.Hooks.PostLink) > 0 {
			app.logger.heading("Running post-link hooks...")
			if err := app.runHooks(config.Hooks.PostLink); err != nil {
				app.logger.error("Post-link hook failed: %v", err)
			}
		}

		// Process git repositories
		if len(config.Git) > 0 {
			app.logger.heading("Setting up git repositories...")
			for _, path := range slices.Sorted(maps.Keys(config.Git)) {
				app.cloneRepo(path, config.Git[path])
			}
		}

		// Run pre-shell hooks. Shell commands are the destructive part of a
		// section, so a failing pre-hook skips them (and the post-hooks).
		if config.Hooks != nil && len(config.Hooks.PreShell) > 0 {
			app.logger.heading("Running pre-shell hooks...")
			if err := app.runHooks(config.Hooks.PreShell); err != nil {
				app.logger.error("Pre-shell hook failed, skipping shell commands: %v", err)
				continue
			}
		}

		// Process shell commands
		if len(config.Shell) > 0 {
			app.logger.heading("Running shell commands...")
			for _, cmd := range config.Shell {
				app.runShellCommand(cmd)
			}
		}

		// Run post-shell hooks
		if config.Hooks != nil && len(config.Hooks.PostShell) > 0 {
			app.logger.heading("Running post-shell hooks...")
			if err := app.runHooks(config.Hooks.PostShell); err != nil {
				app.logger.error("Post-shell hook failed: %v", err)
			}
		}
	}

	app.logger.summary()
	return app.failureError()
}

// declaredTargets collects every link target across all configs, so duplicate
// removal can never delete a path the user explicitly manages.
func (app *App) declaredTargets(configs []Config) map[string]bool {
	targets := make(map[string]bool)

	for _, config := range configs {
		for target := range config.Link {
			path, err := filepath.Abs(expandPath(target, app.homeDir))
			if err != nil {
				continue
			}
			targets[path] = true
		}
	}

	return targets
}

func (app *App) createDirectory(dir string) {
	dirPath := expandPath(dir, app.homeDir)

	exists, isDir, err := checkPathExists(dirPath)
	if err != nil {
		app.logger.error("Error checking directory %s: %v", dirPath, err)
		return
	}

	if exists {
		if isDir {
			app.logger.info("Directory already exists: %s", dirPath)
			return
		}
		app.logger.warn("Path exists but is not a directory: %s", dirPath)
		return
	}

	app.logger.info("Creating directory: %s", dirPath)
	if err := app.logger.execute(func() error {
		return os.MkdirAll(dirPath, 0755)
	}); err != nil {
		app.logger.error("Error creating directory: %v", err)
	} else if !app.dryRun {
		app.logger.success("Created directory: %s", dirPath)
	}
}

func (app *App) createLink(target, source string, opts linkOptions, declared map[string]bool) {
	targetPath := expandPath(target, app.homeDir)
	targetPath, _ = filepath.Abs(targetPath)
	sourcePath := expandSourcePath(source, app.homeDir, app.execDir)
	sourcePath, _ = filepath.Abs(sourcePath)

	app.logger.debug("Processing link: %s → %s", targetPath, sourcePath)

	// Check if source file exists
	exists, _, err := checkPathExists(sourcePath)
	if err != nil {
		app.logger.error("Error checking source path %s: %v", sourcePath, err)
		return
	}
	if !exists {
		app.logger.error("Source path does not exist: %s", sourcePath)
		return
	}

	// Create parent directories if they don't exist
	parentDir := filepath.Dir(targetPath)
	parentExists, isParentDir, _ := checkPathExists(parentDir)
	if !parentExists {
		app.logger.info("Creating parent directory: %s", parentDir)
		app.logger.execute(func() error {
			return os.MkdirAll(parentDir, 0755)
		})
	} else if !isParentDir {
		app.logger.error("Parent path exists but is not a directory: %s", parentDir)
		return
	}

	// Check for duplicates (opt-in: it deletes files)
	if opts.removeDuplicates {
		app.checkForDuplicates(targetPath, sourcePath, declared)
	}

	// Check target path
	targetExists, isTargetDir, _ := checkPathExists(targetPath)
	if targetExists {
		// Check if it's a symlink
		fileInfo, err := os.Lstat(targetPath)
		if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
			currentTarget, err := os.Readlink(targetPath)
			if err == nil {
				// Make currentTarget absolute for comparison
				if !filepath.IsAbs(currentTarget) {
					currentTarget = filepath.Join(filepath.Dir(targetPath), currentTarget)
				}
				currentTarget, _ = filepath.Abs(currentTarget)

				if currentTarget == sourcePath {
					app.logger.info("Symlink already correct: %s", targetPath)
					app.logger.successCount++ // Count as success
					return
				}

				if opts.relink {
					app.logger.warn("Relinking: %s → %s (was: %s)", targetPath, sourcePath, currentTarget)
					app.logger.execute(func() error {
						return os.Remove(targetPath)
					})
				} else {
					app.logger.info("Existing symlink left unchanged: %s → %s", targetPath, currentTarget)
					return
				}
			}
		} else if opts.force {
			// Not a symlink but force is true - back it up before destroying it.
			// If that backup can't be made, leave the file alone: an
			// unrecoverable overwrite is worse than a skipped link.
			if opts.backup {
				if err := app.createBackup(targetPath, isTargetDir); err != nil {
					app.logger.error("Backup failed, refusing to overwrite %s: %v", targetPath, err)
					return
				}
			}
			app.logger.warn("Removing existing path (force=true): %s", targetPath)
			app.logger.execute(func() error {
				return os.RemoveAll(targetPath)
			})
		} else {
			app.logger.warn("Path exists and is not a symlink (use force=true): %s", targetPath)
			return
		}
	}

	// Create symlink
	app.logger.info("Creating symlink: %s → %s", targetPath, sourcePath)
	if err := app.logger.execute(func() error {
		return os.Symlink(sourcePath, targetPath)
	}); err != nil {
		app.logger.error("Error creating symlink: %v", err)
	} else if !app.dryRun {
		app.logger.success("Created symlink: %s", targetPath)
	}
}

// checkForDuplicates removes other symlinks in the target's directory that point
// at the same source. Targets declared anywhere in the config are never touched:
// two config entries sharing one source are a legitimate setup (~/.bashrc and
// ~/.bash_profile, say), and deleting one of them would make the two entries
// destroy each other on every run.
//
// Removals are logged rather than backed up on purpose. A symlink holds no data
// of its own, and backups are keyed by path — copying one here would follow the
// link and overwrite an existing backup of the real file that used to live there.
func (app *App) checkForDuplicates(targetPath, sourcePath string, declared map[string]bool) {
	targetDir := filepath.Dir(targetPath)

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetDir, entry.Name())

		if entryPath == targetPath || declared[entryPath] {
			continue
		}

		if entry.Type()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(entryPath)
			if err != nil {
				continue
			}

			if !filepath.IsAbs(linkDest) {
				linkDest = filepath.Join(targetDir, linkDest)
			}
			linkDest, _ = filepath.Abs(linkDest)

			if linkDest == sourcePath {
				app.logger.warn("Removing duplicate symlink: %s → %s", entryPath, sourcePath)
				app.logger.execute(func() error {
					return os.Remove(entryPath)
				})
			}
		}
	}
}

func (app *App) cloneRepo(path string, repo GitRepo) {
	repoPath := expandPath(path, app.homeDir)
	exists, isDir, err := checkPathExists(repoPath)

	if err != nil {
		app.logger.error("Error checking repository path %s: %v", repoPath, err)
		return
	}

	if exists {
		if !isDir {
			app.logger.warn("Path exists but is not a directory: %s", repoPath)
			return
		}
		app.logger.info("Repository already exists: %s", repoPath)
		return
	}

	description := repo.Description
	if description == "" {
		description = repo.URL
	}

	app.logger.info("Cloning %s to %s", description, repoPath)
	if err := app.logger.execute(func() error {
		cmd := exec.Command("git", "clone", repo.URL, repoPath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %s", err, stderr.String())
		}
		return nil
	}); err != nil {
		app.logger.error("Error cloning repository: %v", err)
	} else if !app.dryRun {
		app.logger.success("Cloned: %s", repoPath)
	}
}

func (app *App) runShellCommand(cmd ShellCommand) {
	description := cmd.Description
	if description == "" {
		description = cmd.Command
	}

	app.logger.info("Running: %s", description)
	app.logger.debug("Command: %s", cmd.Command)

	if err := app.logger.execute(func() error {
		execCmd := buildShellCmd(cmd.Command)
		execCmd.Dir = app.execDir

		var stdout, stderr bytes.Buffer
		execCmd.Stdout = &stdout
		execCmd.Stderr = &stderr

		if cmd.Stdin != "" {
			execCmd.Stdin = strings.NewReader(cmd.Stdin)
		}

		if err := execCmd.Run(); err != nil {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = stdout.String()
			}
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(errMsg))
		}

		if app.verbose && stdout.Len() > 0 {
			app.logger.debug("Output: %s", strings.TrimSpace(stdout.String()))
		}

		return nil
	}); err != nil {
		app.logger.error("Command failed: %v", err)
	} else if !app.dryRun {
		app.logger.success("Executed: %s", description)
	}
}

func (app *App) runHooks(hooks []string) error {
	for _, hook := range hooks {
		app.logger.debug("Running hook: %s", hook)
		if err := app.logger.execute(func() error {
			cmd := buildShellCmd(hook)
			cmd.Dir = app.execDir
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%v: %s", err, stderr.String())
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
