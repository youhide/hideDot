// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// App holds the application state
type App struct {
	logger     *Logger
	configPath string
	execDir    string
	homeDir    string
	backupDir  string
	profile    string
	dryRun     bool
	verbose    bool
	quiet      bool
	noColor    bool
	noBackup   bool
	tmplData   TemplateData
}

// NewApp creates a new application instance
func NewApp() *App {
	return &App{
		backupDir: filepath.Join(os.Getenv("HOME"), ".hidedot-backups"),
	}
}

// Initialize sets up the application
func (app *App) Initialize() error {
	var err error

	app.homeDir, err = os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting home directory: %w", err)
	}

	app.execDir, err = getExecutableDir()
	if err != nil {
		return fmt.Errorf("error getting executable directory: %w", err)
	}

	// Initialize template data
	hostname, _ := os.Hostname()
	app.tmplData = TemplateData{
		Hostname: hostname,
		Username: os.Getenv("USER"),
		HomeDir:  app.homeDir,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Date:     time.Now().Format("2006-01-02"),
	}

	// Create logger
	useColors := supportsColor() && !app.noColor
	app.logger = &Logger{
		dryRun:    app.dryRun,
		useColors: useColors,
		verbose:   app.verbose,
		quiet:     app.quiet,
	}

	return nil
}

// LoadConfigs loads and validates configuration files
func (app *App) LoadConfigs() ([]Config, error) {
	data, err := os.ReadFile(app.configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Expand templates in config
	expandedData, err := app.expandTemplates(string(data))
	if err != nil {
		return nil, fmt.Errorf("error expanding templates: %w", err)
	}

	var configs []Config
	if err := yaml.Unmarshal([]byte(expandedData), &configs); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate and filter by profile
	var filteredConfigs []Config
	for _, cfg := range configs {
		if err := app.validateConfig(cfg); err != nil {
			return nil, fmt.Errorf("config validation error: %w", err)
		}

		// Filter by profile if specified
		if app.profile != "" && cfg.Profile != "" && cfg.Profile != app.profile {
			app.logger.debug("Skipping config with profile '%s' (current: '%s')", cfg.Profile, app.profile)
			continue
		}
		filteredConfigs = append(filteredConfigs, cfg)
	}

	return filteredConfigs, nil
}

// expandTemplates expands Go templates in the config
func (app *App) expandTemplates(content string) (string, error) {
	tmpl, err := template.New("config").Parse(content)
	if err != nil {
		return content, nil // Return original if template parsing fails
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, app.tmplData); err != nil {
		return content, nil // Return original if template execution fails
	}

	return buf.String(), nil
}

// validateConfig validates a configuration
func (app *App) validateConfig(cfg Config) error {
	// Validate link paths
	for target, source := range cfg.Link {
		if target == "" {
			return fmt.Errorf("link target cannot be empty")
		}
		if source == "" {
			return fmt.Errorf("link source cannot be empty for target '%s'", target)
		}
	}

	// Validate git repos
	for path, repo := range cfg.Git {
		if path == "" {
			return fmt.Errorf("git repository path cannot be empty")
		}
		if repo.URL == "" {
			return fmt.Errorf("git repository URL cannot be empty for path '%s'", path)
		}
	}

	// Validate shell commands
	for i, cmd := range cfg.Shell {
		if cmd.Command == "" {
			return fmt.Errorf("shell command at index %d cannot be empty", i)
		}
	}

	return nil
}

// getDefaultOptions resolves the effective link options for a config section.
func (app *App) getDefaultOptions(config Config) (force, relink, backup bool) {
	if config.Defaults == nil {
		return false, false, true // backup enabled by default
	}
	return config.Defaults.Link.Force, config.Defaults.Link.Relink, config.Defaults.Link.Backup
}
