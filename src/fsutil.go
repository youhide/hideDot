// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// buildShellCmd returns a command that runs the given string through the
// platform's shell (cmd on Windows, bash elsewhere).
func buildShellCmd(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("bash", "-c", command)
}

// getWorkingDir returns the directory relative link sources are resolved
// against: the directory hidedot was invoked from, not where the binary lives.
func getWorkingDir() (string, error) {
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

// movePath moves src to dst, falling back to copy-then-delete when the two live
// on different filesystems (os.Rename fails with EXDEV there).
func movePath(src, dst string, isDir bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	if isDir {
		if err := copyDir(src, dst); err != nil {
			return err
		}
	} else if err := copyFile(src, dst); err != nil {
		return err
	}

	return os.RemoveAll(src)
}

// writeFileAtomic writes through a temp file in the same directory, so an
// interrupted write can never leave a truncated config behind.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Preserve the original file's mode when replacing an existing file.
	if info, err := os.Stat(path); err == nil {
		if err := os.Chmod(tmpName, info.Mode()); err != nil {
			return err
		}
	}

	return os.Rename(tmpName, path)
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
