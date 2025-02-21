package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

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

// Config represents a single configuration section
type Config struct {
	Defaults *struct {
		Link struct {
			Relink bool `yaml:"relink"`
			Force  bool `yaml:"force"`
		} `yaml:"link"`
	} `yaml:"defaults,omitempty"`
	Link   map[string]string  `yaml:"link,omitempty"`
	Create []string           `yaml:"create,omitempty"`
	Git    map[string]GitRepo `yaml:"git,omitempty"`
	Shell  [][]interface{}    `yaml:"shell,omitempty"`
}

// GitRepo represents a git repository configuration
type GitRepo struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// SymlinkInfo stores information about a symlink
type SymlinkInfo struct {
	Target string // Where the symlink is located
	Source string // Where it points to
}

// Logger handles logging with dry run and color support
type Logger struct {
	dryRun       bool
	useColors    bool
	errorCount   int
	successCount int
}

func (l *Logger) log(format string, args ...interface{}) {
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
	if l.useColors {
		l.log(Green+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) info(format string, args ...interface{}) {
	if l.useColors {
		l.log(Blue+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) warn(format string, args ...interface{}) {
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
	if l.useColors {
		fmt.Printf("\n"+BoldCyan+format+Reset+"\n", args...)
	} else {
		fmt.Printf("\n"+format+"\n", args...)
	}
}

func (l *Logger) summary() {
	if l.useColors {
		fmt.Printf("\n"+BoldGreen+"%d operations successful"+Reset+", "+BoldRed+"%d errors encountered"+Reset+"\n",
			l.successCount, l.errorCount)
	} else {
		fmt.Printf("\n%d operations successful, %d errors encountered\n",
			l.successCount, l.errorCount)
	}
}

func (l *Logger) execute(action func() error) error {
	if l.dryRun {
		return nil
	}
	err := action()
	if err != nil {
		l.errorCount++
	}
	return err
}

// getDefaultOptions returns safe default values even if the config structure is nil
func getDefaultOptions(config Config) (force bool, relink bool) {
	if config.Defaults == nil {
		return false, false
	}
	return config.Defaults.Link.Force, config.Defaults.Link.Relink
}

// getExecutableDir returns the directory containing the executable
func getExecutableDir() (string, error) {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe), nil
	}
	return os.Getwd()
}

// checkForDuplicates checks if there are duplicate symlinks for a specific target
func checkForDuplicates(targetPath string, sourcePath string, logger *Logger) {
	// Get the directory containing the target
	targetDir := filepath.Dir(targetPath)

	// Read the directory entries
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return
	}

	// Find all symlinks in this directory that point to our source
	var duplicates []string
	for _, entry := range entries {
		entryPath := filepath.Join(targetDir, entry.Name())

		// Skip if it's our target path
		if entryPath == targetPath {
			continue
		}

		// Check if it's a symlink
		if entry.Type()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(entryPath)
			if err != nil {
				continue
			}

			// Make linkDest absolute if it's relative
			if !filepath.IsAbs(linkDest) {
				linkDest = filepath.Join(targetDir, linkDest)
			}
			linkDest, _ = filepath.Abs(linkDest)

			// If this symlink points to our source, mark it for removal
			if linkDest == sourcePath {
				duplicates = append(duplicates, entryPath)
			}
		}
	}

	// Remove any duplicates found
	for _, dup := range duplicates {
		logger.warn("Found duplicate symlink: %s -> %s", dup, sourcePath)
		logger.execute(func() error {
			return os.Remove(dup)
		})
	}
}

// checkPathExists checks if a path exists and whether it's a directory
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

// expandPath expands ~ to the user's home directory
func expandPath(path string, home string) string {
	if path == "~" {
		return home
	}
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(home, path[2:])
	}
	return path
}

// function to handle source path expansion
func expandSourcePath(path string, home string, execDir string) string {
	// First expand any home directory references
	path = expandPath(path, home)

	// If the path is absolute after home expansion, use it as is
	if filepath.IsAbs(path) {
		return path
	}

	// Otherwise, treat it as relative to the executable directory
	return filepath.Join(execDir, path)
}

// supportsColor checks if the terminal supports color output
func supportsColor() bool {
	// On Windows, check if we're running in a terminal that supports ANSI
	if runtime.GOOS == "windows" {
		// Check for common terminals that support color
		if os.Getenv("TERM") != "" || os.Getenv("ConEmuANSI") == "ON" || os.Getenv("ANSICON") != "" {
			return true
		}
		return false
	}

	// For other platforms, check if stdout is a terminal
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func main() {
	dryRun := flag.Bool("dry-run", false, "Show what would be done without making actual changes")
	configFile := flag.String("config", "hidedot.conf.yaml", "Path to config file")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	flag.Parse()

	// Determine if we should use colors
	useColors := supportsColor() && !*noColor

	logger := &Logger{
		dryRun:    *dryRun,
		useColors: useColors,
	}

	currentDir, err := os.Getwd()
	if err != nil {
		logger.error("Error getting current directory: %v", err)
		os.Exit(1)
	}

	// Use provided config path or default to current directory
	configPath := *configFile
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(currentDir, configPath)
	}

	execDir, err := getExecutableDir()
	if err != nil {
		logger.error("Error getting executable directory: %v", err)
		os.Exit(1)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.error("Error reading config file: %v", err)
		os.Exit(1)
	}

	var configs []Config
	if err := yaml.Unmarshal(data, &configs); err != nil {
		logger.error("Error parsing config file: %v", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.error("Error getting home directory: %v", err)
		os.Exit(1)
	}

	for _, config := range configs {
		force, relink := getDefaultOptions(config)
		if config.Defaults != nil {
			logger.info("Setting defaults: force=%v, relink=%v", force, relink)
		}

		// Process link creation
		if len(config.Link) > 0 {
			logger.heading("Creating links...")
			for target, source := range config.Link {
				targetPath := expandPath(target, home)
				sourcePath := expandSourcePath(source, home, execDir)
				sourcePath, _ = filepath.Abs(sourcePath)

				// Check if source file exists
				exists, _, err := checkPathExists(sourcePath)
				if err != nil {
					logger.error("Error checking source path %s: %v", sourcePath, err)
					continue
				}
				if !exists {
					logger.error("Source path does not exist: %s", sourcePath)
					continue
				}

				// Create parent directories if they don't exist
				parentDir := filepath.Dir(targetPath)
				parentExists, isParentDir, _ := checkPathExists(parentDir)
				if !parentExists {
					logger.info("Creating parent directory: %s", parentDir)
					logger.execute(func() error {
						return os.MkdirAll(parentDir, 0755)
					})
				} else if !isParentDir {
					logger.error("Error: Parent path exists but is not a directory: %s", parentDir)
					continue
				}

				// Check for duplicates before handling the target
				checkForDuplicates(targetPath, sourcePath, logger)

				// Check target path
				targetExists, isTargetDir, _ := checkPathExists(targetPath)
				if targetExists {
					if isTargetDir {
						logger.warn("Target exists and is a directory: %s", targetPath)
					}

					// Check if it's a symlink
					fileInfo, err := os.Lstat(targetPath)
					if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
						currentTarget, err := os.Readlink(targetPath)
						if err == nil {
							if relink && currentTarget != sourcePath {
								logger.warn("Relinking incorrect symlink: %s -> %s (currently: %s)", targetPath, sourcePath, currentTarget)
								logger.execute(func() error {
									return os.Remove(targetPath)
								})
							} else if !relink {
								// Change this line to just state what's happening without showing the value
								logger.info("Existing symlink left unchanged: %s -> %s", targetPath, currentTarget)
								continue
							}
						}
					} else if force {
						// Not a symlink but force is true
						logger.warn("Removing existing path (force=true): %s", targetPath)
						logger.execute(func() error {
							return os.RemoveAll(targetPath)
						})
					} else {
						// Not a symlink and force is false
						logger.warn("Path exists and is not a symlink (force=false): %s", targetPath)
						continue
					}
				}

				// Create symlink
				logger.info("Creating symlink: %s -> %s", targetPath, sourcePath)
				if err := logger.execute(func() error {
					return os.Symlink(sourcePath, targetPath)
				}); err != nil {
					logger.error("Error creating symlink: %v", err)
				} else if !*dryRun {
					logger.success("Successfully created symlink: %s", targetPath)
				}
			}
		}

		// Process directory creation
		if len(config.Create) > 0 {
			logger.heading("Creating directories...")
			for _, dir := range config.Create {
				dirPath := expandPath(dir, home)

				exists, isDir, err := checkPathExists(dirPath)
				if err != nil {
					logger.error("Error checking directory %s: %v", dirPath, err)
					continue
				}

				if exists {
					if isDir {
						logger.info("Directory already exists: %s", dirPath)
						continue
					} else {
						logger.warn("Path exists but is not a directory: %s", dirPath)
						continue
					}
				}

				logger.info("Creating directory: %s", dirPath)
				if err := logger.execute(func() error {
					return os.MkdirAll(dirPath, 0755)
				}); err != nil {
					logger.error("Error creating directory: %v", err)
				} else if !*dryRun {
					logger.success("Successfully created directory: %s", dirPath)
				}
			}
		}

		// Process git repositories
		if len(config.Git) > 0 {
			logger.heading("Setting up git repositories...")
			for path, repo := range config.Git {
				repoPath := expandPath(path, home)
				exists, isDir, err := checkPathExists(repoPath)

				if err != nil {
					logger.error("Error checking repository path %s: %v", repoPath, err)
					continue
				}

				if exists {
					if !isDir {
						logger.warn("Path exists but is not a directory: %s", repoPath)
						continue
					}
					logger.info("Repository already exists at %s", repoPath)
					continue
				}

				logger.info("Cloning %s (%s) to %s", repo.Description, repo.URL, repoPath)
				if err := logger.execute(func() error {
					cmd := exec.Command("git", "clone", repo.URL, repoPath)
					return cmd.Run()
				}); err != nil {
					logger.error("Error cloning repository: %v", err)
				} else if !*dryRun {
					logger.success("Successfully cloned repository: %s", repoPath)
				}
			}
		}

		// Process shell commands
		if len(config.Shell) > 0 {
			logger.heading("Running shell commands...")
			for _, cmd := range config.Shell {
				if len(cmd) >= 2 {
					command := cmd[0].(string)
					description := cmd[1].(string)
					logger.info("Running: %s (%s)", command, description)
					if err := logger.execute(func() error {
						execCmd := exec.Command("bash", "-c", command)
						execCmd.Dir = execDir
						return execCmd.Run()
					}); err != nil {
						logger.error("Error running command: %v", err)
					} else if !*dryRun {
						logger.success("Successfully executed: %s", description)
					}
				}
			}
		}
	}

	// Print a summary
	logger.summary()
}
