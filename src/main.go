package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// Logger handles logging with dry run support
type Logger struct {
	dryRun bool
}

func (l *Logger) log(format string, args ...interface{}) {
	prefix := "==>"
	if l.dryRun {
		prefix = "[DRY RUN] ==>"
	}
	fmt.Printf(prefix+" "+format+"\n", args...)
}

func (l *Logger) execute(action func() error) error {
	if l.dryRun {
		return nil
	}
	return action()
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
		logger.log("Found duplicate symlink: %s -> %s", dup, sourcePath)
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

func main() {
	dryRun := flag.Bool("dry-run", false, "Show what would be done without making actual changes")
	configFile := flag.String("config", "hidedot.conf.yaml", "Path to config file")
	flag.Parse()

	logger := &Logger{dryRun: *dryRun}

	currentDir, err := os.Getwd()
	if err != nil {
		logger.log("Error getting current directory: %v", err)
		os.Exit(1)
	}

	// Use provided config path or default to current directory
	configPath := *configFile
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(currentDir, configPath)
	}

	execDir, err := getExecutableDir()
	if err != nil {
		logger.log("Error getting executable directory: %v", err)
		os.Exit(1)
	}

	// After configPath is set:
	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.log("Error reading config file: %v", err)
		os.Exit(1)
	}

	var configs []Config
	if err := yaml.Unmarshal(data, &configs); err != nil {
		logger.log("Error parsing config file: %v", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.log("Error getting home directory: %v", err)
		os.Exit(1)
	}

	for _, config := range configs {
		force, relink := getDefaultOptions(config)
		if config.Defaults != nil {
			logger.log("Setting defaults: force=%v, relink=%v", force, relink)
		}

		// Process link creation
		if len(config.Link) > 0 {
			logger.log("Creating links...")
			for target, source := range config.Link {
				targetPath := expandPath(target, home)
				sourcePath := filepath.Join(execDir, source)
				sourcePath, _ = filepath.Abs(sourcePath)

				// Check if source file exists
				exists, _, err := checkPathExists(sourcePath)
				if err != nil {
					logger.log("Error checking source path %s: %v", sourcePath, err)
					continue
				}
				if !exists {
					logger.log("Source path does not exist: %s", sourcePath)
					continue
				}

				// Create parent directories if they don't exist
				parentDir := filepath.Dir(targetPath)
				parentExists, isParentDir, _ := checkPathExists(parentDir)
				if !parentExists {
					logger.log("Creating parent directory: %s", parentDir)
					logger.execute(func() error {
						return os.MkdirAll(parentDir, 0755)
					})
				} else if !isParentDir {
					logger.log("Error: Parent path exists but is not a directory: %s", parentDir)
					continue
				}

				// Check for duplicates before handling the target
				checkForDuplicates(targetPath, sourcePath, logger)

				// Check target path
				targetExists, isTargetDir, _ := checkPathExists(targetPath)
				if targetExists {
					if isTargetDir {
						logger.log("Target exists and is a directory: %s", targetPath)
					}

					// Check if it's a symlink
					fileInfo, err := os.Lstat(targetPath)
					if err == nil && fileInfo.Mode()&os.ModeSymlink != 0 {
						currentTarget, err := os.Readlink(targetPath)
						if err == nil {
							if relink && currentTarget != sourcePath {
								logger.log("Relinking incorrect symlink: %s -> %s (currently: %s)", targetPath, sourcePath, currentTarget)
								logger.execute(func() error {
									return os.Remove(targetPath)
								})
							} else if !relink {
								// Change this line to just state what's happening without showing the value
								logger.log("Existing symlink left unchanged: %s -> %s", targetPath, currentTarget)
								continue
							}
						}
					} else if force {
						// Not a symlink but force is true
						logger.log("Removing existing path (force=true): %s", targetPath)
						logger.execute(func() error {
							return os.RemoveAll(targetPath)
						})
					} else {
						// Not a symlink and force is false
						logger.log("Path exists and is not a symlink (force=false): %s", targetPath)
						continue
					}
				}

				// Create symlink
				logger.log("Creating symlink: %s -> %s", targetPath, sourcePath)
				if !*dryRun {
					if err := os.Symlink(sourcePath, targetPath); err != nil {
						logger.log("Error creating symlink: %v", err)
					}
				}
			}
		}

		// Process directory creation
		if len(config.Create) > 0 {
			logger.log("Creating directories...")
			for _, dir := range config.Create {
				dirPath := expandPath(dir, home)

				exists, isDir, err := checkPathExists(dirPath)
				if err != nil {
					logger.log("Error checking directory %s: %v", dirPath, err)
					continue
				}

				if exists {
					if isDir {
						logger.log("Directory already exists: %s", dirPath)
						continue
					} else {
						logger.log("Path exists but is not a directory: %s", dirPath)
						continue
					}
				}

				logger.log("Creating directory: %s", dirPath)
				logger.execute(func() error {
					return os.MkdirAll(dirPath, 0755)
				})
			}
		}

		// Process git repositories
		if len(config.Git) > 0 {
			logger.log("Setting up git repositories...")
			for path, repo := range config.Git {
				repoPath := expandPath(path, home)
				exists, isDir, err := checkPathExists(repoPath)

				if err != nil {
					logger.log("Error checking repository path %s: %v", repoPath, err)
					continue
				}

				if exists {
					if !isDir {
						logger.log("Path exists but is not a directory: %s", repoPath)
						continue
					}
					logger.log("Repository already exists at %s", repoPath)
					continue
				}

				logger.log("Cloning %s (%s) to %s", repo.Description, repo.URL, repoPath)
				if !*dryRun {
					cmd := exec.Command("git", "clone", repo.URL, repoPath)
					if err := cmd.Run(); err != nil {
						logger.log("Error cloning repository: %v", err)
					}
				}
			}
		}

		// Process shell commands
		if len(config.Shell) > 0 {
			logger.log("Running shell commands...")
			for _, cmd := range config.Shell {
				if len(cmd) >= 2 {
					command := cmd[0].(string)
					description := cmd[1].(string)
					logger.log("Running: %s (%s)", command, description)
					if !*dryRun {
						execCmd := exec.Command("bash", "-c", command)
						execCmd.Dir = execDir
						if err := execCmd.Run(); err != nil {
							logger.log("Error running command: %v", err)
						}
					}
				}
			}
		}
	}
}
