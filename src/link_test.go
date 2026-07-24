// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultOptions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want linkOptions
	}{
		{
			name: "no defaults block",
			src:  "- link: {}\n",
			want: linkOptions{backup: true},
		},
		{
			// The regression that matters: declaring defaults without a
			// backup key used to disable backups entirely.
			name: "defaults without a backup key still backs up",
			src:  "- defaults:\n    link:\n      force: true\n      relink: true\n",
			want: linkOptions{force: true, relink: true, backup: true},
		},
		{
			name: "backup explicitly disabled",
			src:  "- defaults:\n    link:\n      force: true\n      backup: false\n",
			want: linkOptions{force: true},
		},
		{
			name: "duplicate removal is opt-in",
			src:  "- defaults:\n    link:\n      remove_duplicates: true\n",
			want: linkOptions{backup: true, removeDuplicates: true},
		},
	}

	app := &App{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs := mustParseConfigs(t, tt.src)
			if got := app.getDefaultOptions(configs[0]); got != tt.want {
				t.Errorf("getDefaultOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("--no-backup wins over the config", func(t *testing.T) {
		noBackupApp := &App{noBackup: true}
		configs := mustParseConfigs(t, "- defaults:\n    link:\n      backup: true\n")
		if got := noBackupApp.getDefaultOptions(configs[0]); got.backup {
			t.Error("--no-backup should disable backups")
		}
	})
}

func TestCreateLink(t *testing.T) {
	t.Run("creates a missing symlink", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")

		app.createLink(target, source, linkOptions{backup: true}, nil)

		dest, err := os.Readlink(target)
		if err != nil {
			t.Fatalf("expected a symlink at %s: %v", target, err)
		}
		if dest != source {
			t.Errorf("symlink points at %q, want %q", dest, source)
		}
	})

	t.Run("leaves a regular file alone without force", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")
		writeTestFile(t, target, "precious")

		app.createLink(target, source, linkOptions{backup: true}, nil)

		if got := readTestFile(t, target); got != "precious" {
			t.Errorf("target was modified without force: %q", got)
		}
	})

	t.Run("backs up before overwriting with force", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")
		writeTestFile(t, target, "precious")

		app.createLink(target, source, linkOptions{force: true, backup: true}, nil)

		if _, err := os.Readlink(target); err != nil {
			t.Fatalf("expected a symlink at %s: %v", target, err)
		}
		if got := readTestFile(t, app.getBackupPath(target)); got != "precious" {
			t.Errorf("backup content = %q, want %q", got, "precious")
		}
	})

	t.Run("relinks a symlink pointing elsewhere", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		other := filepath.Join(app.execDir, "other")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")
		writeTestFile(t, other, "other")
		if err := os.Symlink(other, target); err != nil {
			t.Fatal(err)
		}

		app.createLink(target, source, linkOptions{relink: true, backup: true}, nil)

		dest, err := os.Readlink(target)
		if err != nil {
			t.Fatal(err)
		}
		if dest != source {
			t.Errorf("symlink points at %q, want %q", dest, source)
		}
	})

	t.Run("keeps an existing symlink when relink is off", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		other := filepath.Join(app.execDir, "other")
		target := filepath.Join(app.homeDir, ".zshrc")
		writeTestFile(t, source, "config")
		writeTestFile(t, other, "other")
		if err := os.Symlink(other, target); err != nil {
			t.Fatal(err)
		}

		app.createLink(target, source, linkOptions{backup: true}, nil)

		dest, err := os.Readlink(target)
		if err != nil {
			t.Fatal(err)
		}
		if dest != other {
			t.Errorf("symlink was changed to %q", dest)
		}
	})

	t.Run("missing source is reported as an error", func(t *testing.T) {
		app := newTestApp(t)
		target := filepath.Join(app.homeDir, ".zshrc")

		app.createLink(target, filepath.Join(app.execDir, "nope"), linkOptions{backup: true}, nil)

		if app.logger.errorCount == 0 {
			t.Error("expected an error for a missing source")
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Error("no link should have been created")
		}
	})
}

func TestCheckForDuplicates(t *testing.T) {
	t.Run("removes an undeclared duplicate", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "zshrc")
		target := filepath.Join(app.homeDir, ".zshrc")
		stale := filepath.Join(app.homeDir, ".zshrc.old")
		writeTestFile(t, source, "config")
		if err := os.Symlink(source, stale); err != nil {
			t.Fatal(err)
		}

		app.checkForDuplicates(target, source, nil)

		if _, err := os.Lstat(stale); !os.IsNotExist(err) {
			t.Error("stale duplicate should have been removed")
		}
	})

	t.Run("never removes a declared target", func(t *testing.T) {
		app := newTestApp(t)
		source := filepath.Join(app.execDir, "shellrc")
		target := filepath.Join(app.homeDir, ".bashrc")
		sibling := filepath.Join(app.homeDir, ".bash_profile")
		writeTestFile(t, source, "config")
		if err := os.Symlink(source, sibling); err != nil {
			t.Fatal(err)
		}

		app.checkForDuplicates(target, source, map[string]bool{sibling: true})

		if _, err := os.Lstat(sibling); err != nil {
			t.Errorf("declared target was removed: %v", err)
		}
	})
}

// Two config entries sharing one source used to delete each other on every run,
// leaving whichever was processed last as the only survivor.
func TestRunLinkKeepsTargetsSharingASource(t *testing.T) {
	app := newTestApp(t)
	writeTestFile(t, filepath.Join(app.execDir, "shellrc"), "config")

	configs := mustParseConfigs(t, `- defaults:
    link:
      force: true
      relink: true
      remove_duplicates: true
  link:
    ~/.bashrc: ./shellrc
    ~/.bash_profile: ./shellrc
`)

	for run := 1; run <= 2; run++ {
		if err := app.RunLink(configs); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	for _, name := range []string{".bashrc", ".bash_profile"} {
		if _, err := os.Lstat(filepath.Join(app.homeDir, name)); err != nil {
			t.Errorf("%s is gone after two runs: %v", name, err)
		}
	}
}

func TestRunLinkReportsFailures(t *testing.T) {
	app := newTestApp(t)
	configs := mustParseConfigs(t, "- link:\n    ~/.zshrc: ./missing\n")

	if err := app.RunLink(configs); err == nil {
		t.Error("expected a non-nil error so the process exits non-zero")
	}
}

func TestRunLinkAbortsSectionOnPreHookFailure(t *testing.T) {
	app := newTestApp(t)
	writeTestFile(t, filepath.Join(app.execDir, "zshrc"), "config")

	configs := mustParseConfigs(t, `- hooks:
    pre_link:
      - exit 1
  link:
    ~/.zshrc: ./zshrc
`)

	if err := app.RunLink(configs); err == nil {
		t.Error("expected a failing pre-link hook to be reported")
	}
	if _, err := os.Lstat(filepath.Join(app.homeDir, ".zshrc")); !os.IsNotExist(err) {
		t.Error("links should not be created after a failed pre-link hook")
	}
}

func TestRunUnlinkRestoresBackup(t *testing.T) {
	app := newTestApp(t)
	source := filepath.Join(app.execDir, "zshrc")
	target := filepath.Join(app.homeDir, ".zshrc")
	writeTestFile(t, source, "config")
	writeTestFile(t, target, "precious")

	configs := mustParseConfigs(t, `- defaults:
    link:
      force: true
  link:
    ~/.zshrc: ./zshrc
`)

	if err := app.RunLink(configs); err != nil {
		t.Fatal(err)
	}
	if err := app.RunUnlink(configs, true); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("target was not restored: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("restored target is still a symlink")
	}
	if got := readTestFile(t, target); got != "precious" {
		t.Errorf("restored content = %q, want %q", got, "precious")
	}
}

func TestRunBackupRecordsManifest(t *testing.T) {
	app := newTestApp(t)
	target := filepath.Join(app.homeDir, ".zshrc")
	writeTestFile(t, target, "precious")

	configs := mustParseConfigs(t, "- link:\n    ~/.zshrc: ./zshrc\n")
	if err := app.RunBackup(configs); err != nil {
		t.Fatal(err)
	}

	manifest := app.readManifest()
	entry, ok := manifest[filepath.Base(app.getBackupPath(target))]
	if !ok {
		t.Fatalf("backup not recorded in manifest: %+v", manifest)
	}
	if entry.OriginalPath != target {
		t.Errorf("manifest path = %q, want %q", entry.OriginalPath, target)
	}
}

// copyDir merges into whatever is already there, so a stale backup must be
// cleared before a directory is backed up again.
func TestCreateBackupReplacesStaleDirectory(t *testing.T) {
	app := newTestApp(t)
	target := filepath.Join(app.homeDir, ".config")
	writeTestFile(t, filepath.Join(target, "keep"), "keep")
	writeTestFile(t, filepath.Join(target, "gone"), "gone")

	if err := app.createBackup(target, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(target, "gone")); err != nil {
		t.Fatal(err)
	}
	if err := app.createBackup(target, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(app.getBackupPath(target), "gone")); !os.IsNotExist(err) {
		t.Error("deleted file survived in the refreshed backup")
	}
}
