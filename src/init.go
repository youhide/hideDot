// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
)

// defaultConfigTemplate is the starter config written by `hidedot init`.
const defaultConfigTemplate = `- defaults:
    link:
      relink: true
      force: true
      backup: true  # Enable automatic backups

  # Create directories before linking
  create:
    - ~/.config

  # Manage symlinks (target: source relative to this repo)
  link:
    ~/.zshrc: ./zsh/zshrc

  # Clone git repositories
  # git:
  #   ~/.oh-my-zsh:
  #     url: https://github.com/ohmyzsh/ohmyzsh.git
  #     description: "Oh My Zsh"

  # Run shell commands
  # shell:
  #   - [touch ~/.hushlogin, Create hushlogin]

  # Hooks for custom actions
  # hooks:
  #   post_link:
  #     - echo "Links created successfully!"
`

// RunInit writes a starter config file to app.configPath.
func (app *App) RunInit(force bool) error {
	exists, _, err := checkPathExists(app.configPath)
	if err != nil {
		return fmt.Errorf("error checking config path: %w", err)
	}
	if exists && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", app.configPath)
	}

	app.logger.info("Writing config: %s", app.configPath)
	if err := app.logger.execute(func() error {
		return os.WriteFile(app.configPath, []byte(defaultConfigTemplate), 0644)
	}); err != nil {
		return fmt.Errorf("error writing config: %w", err)
	}
	if !app.dryRun {
		app.logger.success("Created: %s", app.configPath)
	}
	return nil
}
