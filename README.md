# hideDot
A blazing fast dotFiles manager written in Go. Easily manage your dotfiles, symlinks, and system configuration with a simple YAML config.
<p align="center"><img src="hidedot-logo.svg" alt="Description" width="300" height="300" align="center"></p>

## Features

- 🚀 Fast symlink management with subcommands
- 🏠 Home directory path expansion (`~/`)
- 🔄 Git repository cloning
- 🛠️ Shell command execution
- 🔍 Duplicate symlink detection
- 🧪 Dry-run mode
- 🔙 **Automatic backups** before overwriting files
- ↩️ **Unlink & restore** symlinks with backup restoration
- 📋 **Status command** to check symlink health
- 🏷️ **Profiles** for different machines/environments
- 📝 **Templates** with variables (hostname, OS, etc.)
- 🪝 **Hooks** for pre/post operations

## Installation

```bash
brew tap youhide/homebrew-youhide
brew install hidedot
```

## Usage

### Basic Commands

```bash
# Create symlinks (default command)
hidedot
hidedot link

# Check status of all symlinks
hidedot status

# Remove symlinks
hidedot unlink

# Remove symlinks and restore backups
hidedot unlink --restore

# Manage backups
hidedot backup create
hidedot backup list
```

### Configuration

Create `hidedot.conf.yaml`:

```yaml
- defaults:
    link:
      relink: true
      force: true
      backup: true  # Enable automatic backups
  
  # Optional: profile for filtering configs
  profile: personal
  
  # Create directories
  create:
    - ~/.config
    - ~/.local/bin
  
  # Manage symlinks
  link:
    ~/.config/nvim: ~/.mydotfiles/nvim
    ~/.zshrc: ~/.mydotfiles/zsh/zshrc
  
  # Clone git repositories
  git:
    ~/.oh-my-zsh:
      url: https://github.com/ohmyzsh/ohmyzsh.git
      description: "Oh My Zsh"

  # Run shell commands
  shell:
    - [touch ~/.hushlogin, Create hushlogin]
    # Or with stdin support:
    - command: "cat > ~/.config/myapp/config.json"
      description: "Create config file"
      stdin: '{"key": "value"}'

  # Hooks for custom actions
  hooks:
    pre_link:
      - echo "Starting link process..."
    post_link:
      - echo "Links created successfully!"

# Multiple profiles in same file
- profile: work
  link:
    ~/.gitconfig: ~/.mydotfiles/git/gitconfig-work
```

### Using Templates

Templates use Go's text/template syntax with these variables:

```yaml
- link:
    ~/.config/git/config-{{ .Hostname }}: ./git/config
  
  shell:
    - ["echo 'Running on {{ .OS }}/{{ .Arch }}'", "Show system info"]
```

Available template variables:
- `{{ .Hostname }}` - Machine hostname
- `{{ .Username }}` - Current user
- `{{ .HomeDir }}` - Home directory path
- `{{ .OS }}` - Operating system (darwin, linux, windows)
- `{{ .Arch }}` - Architecture (amd64, arm64)
- `{{ .Date }}` - Current date (YYYY-MM-DD)

## Options

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file (default: hidedot.conf.yaml) |
| `--profile` | `-p` | Only apply configs matching this profile |
| `--dry-run` | `-n` | Show what would be done without making changes |
| `--verbose` | `-v` | Enable verbose output with debug info |
| `--quiet` | `-q` | Only show errors |
| `--no-color` | | Disable colored output |
| `--no-backup` | | Disable automatic backups |

## Subcommands

| Command | Description |
|---------|-------------|
| `link` | Create symlinks from config (default) |
| `status` | Show status of all symlinks (OK, MISSING, BROKEN, MISMATCH) |
| `unlink` | Remove symlinks (use `--restore` to restore backups) |
| `backup create` | Manually create backups of all linked files |
| `backup list` | List available backups |

## Examples

```bash
# Apply only work profile configs
hidedot --profile work

# Preview changes without applying
hidedot --dry-run

# Verbose output for debugging
hidedot -v

# Use custom config file
hidedot -c ~/my-dotfiles/config.yaml

# Quick status check
hidedot status

# Remove all symlinks and restore original files
hidedot unlink --restore
```

## Documentation

For detailed documentation, check out our [Wiki](../../wiki).

## License

This project is licensed under the **GNU General Public License v3.0** - see the [LICENSE](LICENSE) file for details.

This means you are free to use, modify, and distribute this software, as long as any derivative works are also licensed under GPL-3.0.
