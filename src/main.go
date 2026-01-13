package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Version information (injected at build time via ldflags)
var Version = "dev"

// ANSI color codes
const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Red        = "\033[31m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Magenta    = "\033[35m"
	Cyan       = "\033[36m"
	White      = "\033[37m"
	BoldRed    = "\033[1;31m"
	BoldGreen  = "\033[1;32m"
	BoldYellow = "\033[1;33m"
	BoldBlue   = "\033[1;34m"
	BoldCyan   = "\033[1;36m"
)

// LinkStatus represents the state of a symlink
type LinkStatus int

const (
	StatusOK LinkStatus = iota
	StatusMissing
	StatusBroken
	StatusMismatch
	StatusNotSymlink
)

func (s LinkStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusMissing:
		return "MISSING"
	case StatusBroken:
		return "BROKEN"
	case StatusMismatch:
		return "MISMATCH"
	case StatusNotSymlink:
		return "NOT_SYMLINK"
	default:
		return "UNKNOWN"
	}
}

// Config represents a single configuration section
type Config struct {
	Defaults *struct {
		Link struct {
			Relink bool `yaml:"relink"`
			Force  bool `yaml:"force"`
			Backup bool `yaml:"backup"`
		} `yaml:"link"`
	} `yaml:"defaults,omitempty"`
	Profile string             `yaml:"profile,omitempty"`
	Link    map[string]string  `yaml:"link,omitempty"`
	Create  []string           `yaml:"create,omitempty"`
	Git     map[string]GitRepo `yaml:"git,omitempty"`
	Shell   []ShellCommand     `yaml:"shell,omitempty"`
	Hooks   *Hooks             `yaml:"hooks,omitempty"`
}

// ShellCommand can be either [command, description] or {command, description, stdin}
type ShellCommand struct {
	Command     string
	Description string
	Stdin       string
}

// UnmarshalYAML handles both array and map formats for shell commands
func (s *ShellCommand) UnmarshalYAML(node *yaml.Node) error {
	// Try array format first: [command, description]
	if node.Kind == yaml.SequenceNode {
		var arr []string
		if err := node.Decode(&arr); err != nil {
			return err
		}
		if len(arr) >= 2 {
			s.Command = arr[0]
			s.Description = arr[1]
		}
		return nil
	}

	// Try map format: {command: ..., description: ..., stdin: ...}
	var m struct {
		Command     string `yaml:"command"`
		Description string `yaml:"description"`
		Stdin       string `yaml:"stdin"`
	}
	if err := node.Decode(&m); err != nil {
		return err
	}
	s.Command = m.Command
	s.Description = m.Description
	s.Stdin = m.Stdin
	return nil
}

// Hooks for pre/post operations
type Hooks struct {
	PreLink   []string `yaml:"pre_link,omitempty"`
	PostLink  []string `yaml:"post_link,omitempty"`
	PreShell  []string `yaml:"pre_shell,omitempty"`
	PostShell []string `yaml:"post_shell,omitempty"`
}

// GitRepo represents a git repository configuration
type GitRepo struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// LinkInfo stores detailed information about a link
type LinkInfo struct {
	Target       string
	Source       string
	Status       LinkStatus
	CurrentDest  string
	ErrorMessage string
}

// TemplateData contains variables for template expansion
type TemplateData struct {
	Hostname string
	Username string
	HomeDir  string
	OS       string
	Arch     string
	Date     string
}

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

// Logger handles logging with dry run and color support
type Logger struct {
	dryRun       bool
	useColors    bool
	verbose      bool
	quiet        bool
	errorCount   int
	successCount int
	warnCount    int
}

func (l *Logger) log(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	var prefix string

	if l.dryRun {
		if l.useColors {
			prefix = BoldYellow + "[DRY RUN]" + Reset + " " + BoldCyan + "==>" + Reset
		} else {
			prefix = "[DRY RUN] ==>"
		}
	} else {
		if l.useColors {
			prefix = BoldCyan + "==>" + Reset
		} else {
			prefix = "==>"
		}
	}

	fmt.Printf(prefix+" "+format+"\n", args...)
}

func (l *Logger) success(format string, args ...interface{}) {
	l.successCount++
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Green+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) info(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Blue+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) debug(format string, args ...interface{}) {
	if !l.verbose || l.quiet {
		return
	}
	if l.useColors {
		l.log(Magenta+"[DEBUG] "+format+Reset, args...)
	} else {
		l.log("[DEBUG] "+format, args...)
	}
}

func (l *Logger) warn(format string, args ...interface{}) {
	l.warnCount++
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Yellow+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) error(format string, args ...interface{}) {
	l.errorCount++
	if l.useColors {
		l.log(Red+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) heading(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	if l.useColors {
		fmt.Printf("\n"+BoldCyan+format+Reset+"\n", args...)
	} else {
		fmt.Printf("\n"+format+"\n", args...)
	}
}

func (l *Logger) summary() {
	if l.useColors {
		fmt.Printf("\n"+BoldGreen+"%d successful"+Reset+", "+BoldYellow+"%d warnings"+Reset+", "+BoldRed+"%d errors"+Reset+"\n",
			l.successCount, l.warnCount, l.errorCount)
	} else {
		fmt.Printf("\n%d successful, %d warnings, %d errors\n",
			l.successCount, l.warnCount, l.errorCount)
	}
}

func (l *Logger) execute(action func() error) error {
	if l.dryRun {
		return nil
	}
	return action()
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

// RunLink executes the link command
func (app *App) RunLink(configs []Config) error {
	for _, config := range configs {
		force, relink, backup := app.getDefaultOptions(config)

		if config.Defaults != nil {
			app.logger.info("Settings: force=%v, relink=%v, backup=%v", force, relink, backup)
		}

		// Run pre-link hooks
		if config.Hooks != nil && len(config.Hooks.PreLink) > 0 {
			app.logger.heading("Running pre-link hooks...")
			if err := app.runHooks(config.Hooks.PreLink); err != nil {
				app.logger.error("Pre-link hook failed: %v", err)
			}
		}

		// Process directory creation
		if len(config.Create) > 0 {
			app.logger.heading("Creating directories...")
			for _, dir := range config.Create {
				app.createDirectory(dir)
			}
		}

		// Process link creation
		if len(config.Link) > 0 {
			app.logger.heading("Creating links...")
			for target, source := range config.Link {
				app.createLink(target, source, force, relink, backup && !app.noBackup)
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
			for path, repo := range config.Git {
				app.cloneRepo(path, repo)
			}
		}

		// Run pre-shell hooks
		if config.Hooks != nil && len(config.Hooks.PreShell) > 0 {
			app.logger.heading("Running pre-shell hooks...")
			if err := app.runHooks(config.Hooks.PreShell); err != nil {
				app.logger.error("Pre-shell hook failed: %v", err)
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
	return nil
}

// RunStatus shows the current status of all links
func (app *App) RunStatus(configs []Config) error {
	var allLinks []LinkInfo

	for _, config := range configs {
		for target, source := range config.Link {
			info := app.checkLinkStatus(target, source)
			allLinks = append(allLinks, info)
		}
	}

	// Sort by status (errors first)
	sort.Slice(allLinks, func(i, j int) bool {
		if allLinks[i].Status != allLinks[j].Status {
			return allLinks[i].Status > allLinks[j].Status
		}
		return allLinks[i].Target < allLinks[j].Target
	})

	// Print status table
	app.logger.heading("Link Status Report")
	fmt.Println()

	var okCount, problemCount int

	for _, link := range allLinks {
		var statusIcon, statusColor string
		switch link.Status {
		case StatusOK:
			statusIcon = "✓"
			statusColor = Green
			okCount++
		case StatusMissing:
			statusIcon = "✗"
			statusColor = Yellow
			problemCount++
		case StatusBroken:
			statusIcon = "⚠"
			statusColor = Red
			problemCount++
		case StatusMismatch:
			statusIcon = "≠"
			statusColor = Yellow
			problemCount++
		case StatusNotSymlink:
			statusIcon = "!"
			statusColor = Red
			problemCount++
		}

		if app.logger.useColors {
			fmt.Printf("  %s%s%s %s%s%s → %s\n",
				statusColor, statusIcon, Reset,
				Bold, link.Target, Reset,
				link.Source)
		} else {
			fmt.Printf("  [%s] %s → %s\n",
				link.Status.String(), link.Target, link.Source)
		}

		if link.ErrorMessage != "" {
			fmt.Printf("      %s\n", link.ErrorMessage)
		}
	}

	fmt.Printf("\n%d OK, %d problems\n", okCount, problemCount)
	return nil
}

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

// Helper methods

func (app *App) getDefaultOptions(config Config) (force, relink, backup bool) {
	if config.Defaults == nil {
		return false, false, true // backup enabled by default
	}
	return config.Defaults.Link.Force, config.Defaults.Link.Relink, config.Defaults.Link.Backup
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

func (app *App) createLink(target, source string, force, relink, backup bool) {
	targetPath := expandPath(target, app.homeDir)
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

	// Check for duplicates
	app.checkForDuplicates(targetPath, sourcePath)

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

				if relink {
					app.logger.warn("Relinking: %s → %s (was: %s)", targetPath, sourcePath, currentTarget)
					app.logger.execute(func() error {
						return os.Remove(targetPath)
					})
				} else {
					app.logger.info("Existing symlink left unchanged: %s → %s", targetPath, currentTarget)
					return
				}
			}
		} else if force {
			// Not a symlink but force is true - backup first if enabled
			if backup {
				app.createBackup(targetPath, isTargetDir)
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

func (app *App) checkLinkStatus(target, source string) LinkInfo {
	targetPath := expandPath(target, app.homeDir)
	sourcePath := expandSourcePath(source, app.homeDir, app.execDir)
	sourcePath, _ = filepath.Abs(sourcePath)

	info := LinkInfo{
		Target: targetPath,
		Source: sourcePath,
	}

	// Check if target exists
	fileInfo, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			info.Status = StatusMissing
			info.ErrorMessage = "Symlink does not exist"
			return info
		}
		info.Status = StatusBroken
		info.ErrorMessage = err.Error()
		return info
	}

	// Check if it's a symlink
	if fileInfo.Mode()&os.ModeSymlink == 0 {
		info.Status = StatusNotSymlink
		info.ErrorMessage = "Path exists but is not a symlink"
		return info
	}

	// Read symlink destination
	dest, err := os.Readlink(targetPath)
	if err != nil {
		info.Status = StatusBroken
		info.ErrorMessage = err.Error()
		return info
	}

	// Make dest absolute for comparison
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(targetPath), dest)
	}
	dest, _ = filepath.Abs(dest)
	info.CurrentDest = dest

	// Check if destination exists
	if _, err := os.Stat(targetPath); err != nil {
		info.Status = StatusBroken
		info.ErrorMessage = "Symlink target does not exist"
		return info
	}

	// Check if destination matches expected
	if dest != sourcePath {
		info.Status = StatusMismatch
		info.ErrorMessage = fmt.Sprintf("Points to %s instead of %s", dest, sourcePath)
		return info
	}

	info.Status = StatusOK
	return info
}

func (app *App) checkForDuplicates(targetPath, sourcePath string) {
	targetDir := filepath.Dir(targetPath)

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetDir, entry.Name())

		if entryPath == targetPath {
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
				app.logger.warn("Found duplicate symlink: %s → %s", entryPath, sourcePath)
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
		execCmd := exec.Command("bash", "-c", cmd.Command)
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
			cmd := exec.Command("bash", "-c", hook)
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

// Utility functions

func getExecutableDir() (string, error) {
	// First, try to use the current working directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd, nil
	}

	// Fallback to executable directory
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe), nil
	}

	return "", fmt.Errorf("could not determine working directory")
}

func checkPathExists(path string) (bool, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}
	return true, info.IsDir(), nil
}

func expandPath(path string, home string) string {
	if path == "~" {
		return home
	}
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}

func expandSourcePath(path string, home string, execDir string) string {
	path = expandPath(path, home)

	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(execDir, path)
}

func supportsColor() bool {
	if runtime.GOOS == "windows" {
		if os.Getenv("TERM") != "" || os.Getenv("ConEmuANSI") == "ON" || os.Getenv("ANSICON") != "" {
			return true
		}
		return false
	}

	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CLI Commands

func main() {
	app := NewApp()

	rootCmd := &cobra.Command{
		Use:     "hidedot",
		Short:   "A blazing fast dotFiles manager",
		Long:    "hideDot - Easily manage your dotfiles, symlinks, and system configuration with a simple YAML config.",
		Version: Version,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&app.configPath, "config", "c", "hidedot.conf.yaml", "Path to config file")
	rootCmd.PersistentFlags().StringVarP(&app.profile, "profile", "p", "", "Only apply configs matching this profile")
	rootCmd.PersistentFlags().BoolVarP(&app.dryRun, "dry-run", "n", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolVarP(&app.verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&app.quiet, "quiet", "q", false, "Only show errors")
	rootCmd.PersistentFlags().BoolVar(&app.noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&app.noBackup, "no-backup", false, "Disable automatic backups")

	// Link command (default action)
	linkCmd := &cobra.Command{
		Use:   "link",
		Short: "Create symlinks from config (default command)",
		Long:  "Create symlinks, directories, clone git repos, and run shell commands as defined in your config file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			configs, err := app.LoadConfigs()
			if err != nil {
				return err
			}
			return app.RunLink(configs)
		},
	}

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of all symlinks",
		Long:  "Check and display the current status of all symlinks defined in your config file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			configs, err := app.LoadConfigs()
			if err != nil {
				return err
			}
			return app.RunStatus(configs)
		},
	}

	// Unlink command
	var restoreBackups bool
	unlinkCmd := &cobra.Command{
		Use:   "unlink",
		Short: "Remove symlinks",
		Long:  "Remove all symlinks defined in your config file. Use --restore to restore backups.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			configs, err := app.LoadConfigs()
			if err != nil {
				return err
			}
			return app.RunUnlink(configs, restoreBackups)
		},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Initialize(); err != nil {
				return err
			}
			configs, err := app.LoadConfigs()
			if err != nil {
				return err
			}
			return app.RunBackup(configs)
		},
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

	// Add all commands
	rootCmd.AddCommand(linkCmd, statusCmd, unlinkCmd, backupCmd)

	// Make link the default command when no subcommand is provided
	rootCmd.RunE = linkCmd.RunE

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
