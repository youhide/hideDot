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

> Recent Homebrew versions require you to trust third-party taps before the
> first install. If you see `Refusing to load formula ... from untrusted tap`,
> run `brew trust youhide/youhide` once and re-run the install.

## Usage

### Basic Commands

```bash
# Scaffold a starter config in the current directory
hidedot init

# Adopt existing files: move them into the dotfiles dir, symlink them back
# and add the entries to your config
hidedot adopt ~/.zshrc ~/.config/nvim

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
      relink: true            # Replace symlinks that point somewhere else
      force: true             # Replace files/dirs that are not symlinks
      backup: true            # Automatic backups — on unless set to false
      remove_duplicates: false  # Delete other symlinks pointing at the same source
  
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
| `init` | Create a starter `hidedot.conf.yaml` (use `--force` to overwrite) |
| `adopt <path>...` | Move existing files/dirs into the dotfiles dir, replace them with symlinks and add them to the config |
| `link` | Create symlinks from config (default) |
| `status` | Show status of all symlinks (OK, MISSING, BROKEN, MISMATCH) |
| `unlink` | Remove symlinks (use `--restore` to restore backups) |
| `backup create` | Manually create backups of all linked files |
| `backup list` | List available backups |

### `adopt`

`adopt` takes files you already have and brings them under hideDot's control in one step:
it backs the file up, moves it into your dotfiles directory, replaces the original with a
symlink, and writes the matching `link:` entry into your config — comments and formatting
preserved.

The destination mirrors the file's own location, minus the leading dots:

| Original | Lands in the repo as |
|----------|----------------------|
| `~/.zshrc` | `./zshrc` |
| `~/.config/nvim` | `./config/nvim` |
| `~/.config/git/config` | `./config/git/config` |

```bash
hidedot adopt ~/.zshrc ~/.config/nvim   # several paths at once
hidedot adopt ~/.zshrc --to zsh/zshrc   # choose the destination
hidedot adopt ~/.zshrc --no-config      # print the config entry instead of writing it
hidedot adopt ~/.zshrc --dry-run        # preview the move and the resulting config
```

With `--profile`, the entry is written to the section declaring that profile. If no section
matches, hideDot leaves the file alone and prints the entry instead.

| Flag | Description |
|------|-------------|
| `--to` | Destination inside the dotfiles dir (single path only) |
| `--no-config` | Print the config entry instead of writing it |

## Backups

Before overwriting anything that isn't already a symlink, hideDot copies it to
`~/.hidedot-backups`. Backups are keyed by the original path, so re-running keeps one
current copy per file rather than piling up. If a backup can't be made, hideDot refuses to
overwrite the file.

```bash
hidedot backup create   # back up every linked target
hidedot backup list     # original paths and when they were saved
hidedot unlink --restore
```

Turn it off per run with `--no-backup`, or per config section with `backup: false`.

## Exit codes

`hidedot` exits `1` when any operation fails, so it can be used in scripts and CI:

```bash
hidedot || echo "something did not apply"
```

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
