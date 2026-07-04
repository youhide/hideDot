// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
