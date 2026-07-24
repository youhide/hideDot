// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"os"

	"github.com/spf13/cobra"
)

// Version information (injected at build time via ldflags)
var Version = "dev"

func main() {
	app := NewApp()

	rootCmd := &cobra.Command{
		Use:     "hidedot",
		Short:   "A blazing fast dotFiles manager",
		Long:    "hideDot - Easily manage your dotfiles, symlinks, and system configuration with a simple YAML config.",
		Version: Version,
		// Runtime failures are already reported per item; don't bury them
		// under the full usage text.
		SilenceUsage: true,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&app.configPath, "config", "c", "hidedot.conf.yaml", "Path to config file")
	rootCmd.PersistentFlags().StringVarP(&app.profile, "profile", "p", "", "Only apply configs matching this profile")
	rootCmd.PersistentFlags().BoolVarP(&app.dryRun, "dry-run", "n", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolVarP(&app.verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&app.quiet, "quiet", "q", false, "Only show errors")
	rootCmd.PersistentFlags().BoolVar(&app.noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&app.noBackup, "no-backup", false, "Disable automatic backups")

	// withConfig wraps a command that needs an initialized app and loaded config.
	withConfig := func(run func(configs []Config) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			configs, err := app.LoadConfigs()
			if err != nil {
				return err
			}
			return run(configs)
		}
	}

	// Link command (default action)
	linkCmd := &cobra.Command{
		Use:   "link",
		Short: "Create symlinks from config (default command)",
		Long:  "Create symlinks, directories, clone git repos, and run shell commands as defined in your config file.",
		RunE:  withConfig(app.RunLink),
	}

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of all symlinks",
		Long:  "Check and display the current status of all symlinks defined in your config file.",
		RunE:  withConfig(app.RunStatus),
	}

	// Unlink command
	var restoreBackups bool
	unlinkCmd := &cobra.Command{
		Use:   "unlink",
		Short: "Remove symlinks",
		Long:  "Remove all symlinks defined in your config file. Use --restore to restore backups.",
		RunE: withConfig(func(configs []Config) error {
			return app.RunUnlink(configs, restoreBackups)
		}),
	}
	unlinkCmd.Flags().BoolVarP(&restoreBackups, "restore", "r", false, "Restore files from backup after unlinking")

	// Backup command
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Manage backups",
		Long:  "Create and manage backups of your dotfiles.",
	}

	backupCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create backups of all linked files",
		RunE:  withConfig(app.RunBackup),
	}

	backupListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			return app.RunListBackups()
		},
	}

	backupCmd.AddCommand(backupCreateCmd, backupListCmd)

	// Init command
	var initForce bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter config file",
		Long:  "Write a starter hidedot.conf.yaml (or the path given by --config) to get started.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			return app.RunInit(initForce)
		},
	}
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite an existing config file")

	// Adopt command
	var adoptTo string
	var adoptNoConfig bool
	adoptCmd := &cobra.Command{
		Use:   "adopt <path>...",
		Short: "Move existing files into the dotfiles dir, symlink them and record them in the config",
		Long: "Move existing files or directories into the dotfiles directory, replace them with symlinks pointing back, " +
			"and add the matching entries to your config file.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			return app.RunAdopt(args, adoptTo, !adoptNoConfig)
		},
	}
	adoptCmd.Flags().StringVar(&adoptTo, "to", "", "Destination inside the dotfiles dir (single path only)")
	adoptCmd.Flags().BoolVar(&adoptNoConfig, "no-config", false, "Print the config entry instead of writing it")

	// Add all commands
	rootCmd.AddCommand(linkCmd, statusCmd, unlinkCmd, backupCmd, initCmd, adoptCmd)

	// Make link the default command when no subcommand is provided
	rootCmd.RunE = linkCmd.RunE

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
