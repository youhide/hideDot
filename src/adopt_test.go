// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptDestPath(t *testing.T) {
	home := filepath.Join(string(os.PathSeparator), "home", "user")
	repo := filepath.Join(string(os.PathSeparator), "repo", "dotfiles")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "dotfile in home",
			in:   filepath.Join(home, ".zshrc"),
			want: filepath.Join(repo, "zshrc"),
		},
		{
			name: "nested config directory",
			in:   filepath.Join(home, ".config", "nvim"),
			want: filepath.Join(repo, "config", "nvim"),
		},
		{
			name: "deeply nested file",
			in:   filepath.Join(home, ".config", "git", "config"),
			want: filepath.Join(repo, "config", "git", "config"),
		},
		{
			name: "plain name in home",
			in:   filepath.Join(home, "notes"),
			want: filepath.Join(repo, "notes"),
		},
		{
			name: "outside home keeps its layout",
			in:   filepath.Join(string(os.PathSeparator), "etc", "hosts"),
			want: filepath.Join(repo, "etc", "hosts"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adoptDestPath(tt.in, home, repo); got != tt.want {
				t.Errorf("adoptDestPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAdoptPath(t *testing.T) {
	t.Run("moves the file, symlinks it back and records it", func(t *testing.T) {
		app := newTestApp(t)
		target := filepath.Join(app.homeDir, ".config", "nvim", "init.lua")
		writeTestFile(t, target, "vim.opt.number = true")
		writeTestFile(t, app.configPath, "- link: {}\n")

		if err := app.adoptPath(target, "", true); err != nil {
			t.Fatal(err)
		}

		dest := filepath.Join(app.execDir, "config", "nvim", "init.lua")
		if got := readTestFile(t, dest); got != "vim.opt.number = true" {
			t.Errorf("moved content = %q", got)
		}

		link, err := os.Readlink(target)
		if err != nil {
			t.Fatalf("expected a symlink at %s: %v", target, err)
		}
		if link != dest {
			t.Errorf("symlink points at %q, want %q", link, dest)
		}

		config := readTestFile(t, app.configPath)
		if !strings.Contains(config, "~/.config/nvim/init.lua: ./config/nvim/init.lua") {
			t.Errorf("config entry missing:\n%s", config)
		}
	})

	t.Run("honours --to", func(t *testing.T) {
		app := newTestApp(t)
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, target, "export X=1")
		writeTestFile(t, app.configPath, "- link: {}\n")

		if err := app.adoptPath(target, "zsh/zshrc", true); err != nil {
			t.Fatal(err)
		}

		if got := readTestFile(t, filepath.Join(app.execDir, "zsh", "zshrc")); got != "export X=1" {
			t.Errorf("moved content = %q", got)
		}
		if config := readTestFile(t, app.configPath); !strings.Contains(config, "~/.zshrc: ./zsh/zshrc") {
			t.Errorf("config entry missing:\n%s", config)
		}
	})

	t.Run("refuses a path that is already a symlink", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")
		if err := os.Symlink(source, target); err != nil {
			t.Fatal(err)
		}

		if err := app.adoptPath(target, "", true); err == nil {
			t.Error("expected an error for an already-adopted path")
		}
	})

	t.Run("refuses to overwrite an existing destination", func(t *testing.T) {
		app := newTestApp(t)
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, target, "new")
		writeTestFile(t, filepath.Join(app.execDir, "zshrc"), "existing")

		if err := app.adoptPath(target, "", true); err == nil {
			t.Error("expected an error when the destination exists")
		}
		if got := readTestFile(t, filepath.Join(app.execDir, "zshrc")); got != "existing" {
			t.Errorf("destination was overwritten: %q", got)
		}
	})

	t.Run("dry run changes nothing", func(t *testing.T) {
		app := newTestApp(t)
		app.dryRun = true
		app.logger.dryRun = true
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, target, "export X=1")
		original := "- link: {}\n"
		writeTestFile(t, app.configPath, original)

		if err := app.adoptPath(target, "", true); err != nil {
			t.Fatal(err)
		}

		if got := readTestFile(t, target); got != "export X=1" {
			t.Error("dry run moved the file")
		}
		if got := readTestFile(t, app.configPath); got != original {
			t.Errorf("dry run wrote to the config:\n%s", got)
		}
	})
}

func TestAddLinkToConfig(t *testing.T) {
	t.Run("preserves comments and existing entries", func(t *testing.T) {
		app := newTestApp(t)
		writeTestFile(t, app.configPath, `- defaults:
    link:
      relink: true # keep this comment
  # create directories first
  create:
    - ~/.config
  link:
    ~/.zshrc: ./zsh/zshrc
`)

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		got := readTestFile(t, app.configPath)
		for _, want := range []string{
			"# keep this comment",
			"# create directories first",
			"~/.zshrc: ./zsh/zshrc",
			"~/.vimrc: ./vimrc",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("config lost %q:\n%s", want, got)
			}
		}
	})

	t.Run("creates the link mapping when absent", func(t *testing.T) {
		app := newTestApp(t)
		writeTestFile(t, app.configPath, "- create:\n    - ~/.config\n")

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		configs := mustParseConfigs(t, readTestFile(t, app.configPath))
		if configs[0].Link["~/.vimrc"] != "./vimrc" {
			t.Errorf("entry not added: %+v", configs[0].Link)
		}
	})

	t.Run("updates an existing target instead of duplicating it", func(t *testing.T) {
		app := newTestApp(t)
		writeTestFile(t, app.configPath, "- link:\n    ~/.vimrc: ./old\n")

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		got := readTestFile(t, app.configPath)
		if strings.Contains(got, "./old") {
			t.Errorf("old source kept:\n%s", got)
		}
		configs := mustParseConfigs(t, got)
		if configs[0].Link["~/.vimrc"] != "./vimrc" {
			t.Errorf("entry not updated: %+v", configs[0].Link)
		}
	})

	t.Run("writes into the matching profile section", func(t *testing.T) {
		app := newTestApp(t)
		app.profile = "work"
		writeTestFile(t, app.configPath, `- link:
    ~/.zshrc: ./zshrc
- profile: work
  link:
    ~/.gitconfig: ./git/config-work
`)

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		configs := mustParseConfigs(t, readTestFile(t, app.configPath))
		if _, ok := configs[0].Link["~/.vimrc"]; ok {
			t.Error("entry landed in the unprofiled section")
		}
		if configs[1].Link["~/.vimrc"] != "./vimrc" {
			t.Errorf("entry missing from the work section: %+v", configs[1].Link)
		}
	})

	t.Run("leaves a config it cannot parse untouched", func(t *testing.T) {
		app := newTestApp(t)
		// Valid Go template, invalid YAML: the value opens a flow mapping.
		original := "- link:\n    ~/.gitconfig: {{ .HomeDir }}/git\n"
		writeTestFile(t, app.configPath, original)

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		if got := readTestFile(t, app.configPath); got != original {
			t.Errorf("config was rewritten:\n%s", got)
		}
	})

	t.Run("leaves the config alone when no profile matches", func(t *testing.T) {
		app := newTestApp(t)
		app.profile = "laptop"
		original := "- link:\n    ~/.zshrc: ./zshrc\n"
		writeTestFile(t, app.configPath, original)

		if err := app.addLinkToConfig("~/.vimrc", "./vimrc"); err != nil {
			t.Fatal(err)
		}

		if got := readTestFile(t, app.configPath); got != original {
			t.Errorf("entry was written to an unrelated section:\n%s", got)
		}
	})
}

func TestRunAdoptRejectsToWithMultiplePaths(t *testing.T) {
	app := newTestApp(t)

	if err := app.RunAdopt([]string{"a", "b"}, "dest", true); err == nil {
		t.Error("expected an error when --to is used with several paths")
	}
}
