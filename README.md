# hideDot
A blazing fast dotFiles manager written in Go. Easily manage your dotfiles, symlinks, and system configuration with a simple YAML config.
<p align="center"><img src="hidedot-logo.svg" alt="Description" width="300" height="300" align="center"></p>

## Features

- 🚀 Fast symlink management
- 🏠 Home directory path expansion (`~/`)
- 🔄 Git repository cloning
- 🛠️ Shell command execution
- 🔍 Duplicate symlink detection
- 🧪 Dry-run mode

## Installation

```bash
brew tap youhide/homebrew-youhide
brew install hidedot
```

## Usage

1. Create `hidedot.conf.yaml`:
```yaml
- defaults:
    link:
      relink: true
      force: true
  
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
```

2. Run HideDot:
```bash
hidedot --config path/to/hidedot.conf.yaml
```

## Options

- `--dry-run`: Show what would be done without making changes
- `--config`: Specify config file path (default: hidedot.conf.yaml)

## Documentation

For detailed documentation, check out our [Wiki](../../wiki).

## License

MIT License
