// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RunAdopt moves existing files or directories into the dotfiles directory,
// replaces them with symlinks pointing back, and records them in the config.
func (app *App) RunAdopt(targets []string, to string, writeConfig bool) error {
	if to != "" && len(targets) > 1 {
		return fmt.Errorf("--to takes a single path, but %d were given", len(targets))
	}

	for _, target := range targets {
		if err := app.adoptPath(target, to, writeConfig); err != nil {
			app.logger.error("%v", err)
		}
	}

	return app.failureError()
}

func (app *App) adoptPath(target, to string, writeConfig bool) error {
	targetPath, err := filepath.Abs(expandPath(target, app.homeDir))
	if err != nil {
		return fmt.Errorf("cannot resolve %s: %w", target, err)
	}

	info, err := os.Lstat(targetPath)
	if err != nil {
		return fmt.Errorf("cannot adopt %s: %w", targetPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is already a symlink, nothing to adopt", targetPath)
	}
	isDir := info.IsDir()

	dest := adoptDestPath(targetPath, app.homeDir, app.execDir)
	if to != "" {
		dest, _ = filepath.Abs(expandSourcePath(to, app.homeDir, app.execDir))
	}
	if exists, _, _ := checkPathExists(dest); exists {
		return fmt.Errorf("destination already exists: %s (use --to to choose another)", dest)
	}

	if !app.noBackup {
		if err := app.createBackup(targetPath, isDir); err != nil {
			return fmt.Errorf("backup failed, not adopting %s: %w", targetPath, err)
		}
	}

	app.logger.info("Moving %s → %s", targetPath, dest)
	if err := app.logger.execute(func() error {
		return movePath(targetPath, dest, isDir)
	}); err != nil {
		return fmt.Errorf("error moving path: %w", err)
	}

	app.logger.info("Creating symlink: %s → %s", targetPath, dest)
	if err := app.logger.execute(func() error {
		return os.Symlink(dest, targetPath)
	}); err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}

	if !app.dryRun {
		app.logger.success("Adopted: %s", targetPath)
	}

	linkTarget, linkSource := app.configEntry(targetPath, dest)
	if !writeConfig {
		app.printConfigEntry(linkTarget, linkSource)
		return nil
	}

	return app.addLinkToConfig(linkTarget, linkSource)
}

// adoptDestPath mirrors the adopted path's own layout inside the dotfiles
// directory, dropping the leading dot of each component:
//
//	~/.zshrc             -> <repo>/zshrc
//	~/.config/nvim       -> <repo>/config/nvim
//	~/.config/git/config -> <repo>/config/git/config
//	/etc/hosts           -> <repo>/etc/hosts
//
// Mirroring rather than flattening to the basename keeps ~/.config/nvim and
// ~/nvim from colliding in the repo.
func adoptDestPath(targetPath, homeDir, execDir string) string {
	rel, err := filepath.Rel(homeDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		// Outside $HOME: keep the absolute layout minus volume and leading separator.
		rel = strings.TrimPrefix(targetPath, filepath.VolumeName(targetPath))
		rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	}

	parts := strings.Split(rel, string(os.PathSeparator))
	for i, part := range parts {
		if part != "." && part != ".." {
			parts[i] = strings.TrimPrefix(part, ".")
		}
	}

	return filepath.Join(append([]string{execDir}, parts...)...)
}

// configEntry renders the two config keys for an adopted path, preferring ~ and
// a repo-relative source so the config stays portable across machines.
func (app *App) configEntry(targetPath, dest string) (string, string) {
	linkTarget := targetPath
	if rel, err := filepath.Rel(app.homeDir, targetPath); err == nil && !strings.HasPrefix(rel, "..") {
		linkTarget = "~/" + filepath.ToSlash(rel)
	}

	linkSource := "./" + filepath.ToSlash(filepath.Base(dest))
	if rel, err := filepath.Rel(app.execDir, dest); err == nil && !strings.HasPrefix(rel, "..") {
		linkSource = "./" + filepath.ToSlash(rel)
	}

	return linkTarget, linkSource
}

func (app *App) printConfigEntry(linkTarget, linkSource string) {
	app.logger.heading("Add this to your config:")
	fmt.Printf("  link:\n    %s: %s\n", linkTarget, linkSource)
}

// addLinkToConfig inserts target: source into the config's link mapping while
// leaving comments and formatting untouched. Anything that would risk mangling
// the user's file falls back to printing the entry for them to paste.
func (app *App) addLinkToConfig(linkTarget, linkSource string) error {
	// Read the file as written, not the template-expanded copy LoadConfigs
	// works with: writing that back would bake {{ .Hostname }} and friends into
	// the config permanently.
	raw, err := os.ReadFile(app.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			app.logger.warn("No config at %s — run 'hidedot init' to create one", app.configPath)
			app.printConfigEntry(linkTarget, linkSource)
			return nil
		}
		return fmt.Errorf("error reading config: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		// Legitimate for template-heavy configs: `key: {{ .X }}` is valid Go
		// template but invalid YAML.
		app.logger.warn("Config is not plain YAML (%v), leaving %s untouched", err, app.configPath)
		app.printConfigEntry(linkTarget, linkSource)
		return nil
	}

	section := findConfigSection(&doc, app.profile)
	if section == nil {
		if app.profile != "" {
			app.logger.warn("No config section with profile '%s' in %s, leaving it untouched", app.profile, app.configPath)
		} else {
			app.logger.warn("Could not find a config section to update in %s, leaving it untouched", app.configPath)
		}
		app.printConfigEntry(linkTarget, linkSource)
		return nil
	}

	if err := setLinkEntry(section, linkTarget, linkSource); err != nil {
		return err
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2) // the default of 4 would reindent the whole file
	if err := encoder.Encode(&doc); err != nil {
		return fmt.Errorf("error encoding config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error encoding config: %w", err)
	}

	app.logger.info("Adding to %s: %s: %s", app.configPath, linkTarget, linkSource)
	if app.dryRun {
		app.logger.heading("Config would become:")
		fmt.Println(buf.String())
		return nil
	}

	if err := writeFileAtomic(app.configPath, buf.Bytes()); err != nil {
		return fmt.Errorf("error writing config: %w", err)
	}
	app.logger.success("Updated config: %s", app.configPath)

	return nil
}

// findConfigSection picks the mapping to update: the section matching the active
// profile, or the first section when no profile is set.
func findConfigSection(doc *yaml.Node, profile string) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.SequenceNode {
		return nil
	}

	var first *yaml.Node
	for _, item := range root.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if first == nil {
			first = item
		}
		if profile != "" && mappingValue(item, "profile") == profile {
			return item
		}
	}

	// With an explicit profile, writing into an unrelated section would be
	// worse than not writing at all.
	if profile != "" {
		return nil
	}

	return first
}

// mappingValue returns the scalar value stored under key in a mapping node.
func mappingValue(node *yaml.Node, key string) string {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}

// setLinkEntry adds or updates target: source inside the section's link mapping,
// creating that mapping when the section has none.
func setLinkEntry(section *yaml.Node, target, source string) error {
	var links *yaml.Node
	for i := 0; i+1 < len(section.Content); i += 2 {
		if section.Content[i].Value == "link" {
			links = section.Content[i+1]
			break
		}
	}

	if links == nil {
		section.Content = append(section.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "link"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
		links = section.Content[len(section.Content)-1]
	}

	// An empty `link:` key decodes as a null scalar; turn it into a mapping.
	if links.Kind == yaml.ScalarNode && links.Tag == "!!null" {
		links.Kind = yaml.MappingNode
		links.Tag = "!!map"
		links.Value = ""
	}

	if links.Kind != yaml.MappingNode {
		return fmt.Errorf("'link' in the config is not a mapping, cannot add %s", target)
	}

	for i := 0; i+1 < len(links.Content); i += 2 {
		if links.Content[i].Value == target {
			links.Content[i+1].Value = source
			links.Content[i+1].Tag = "!!str"
			links.Content[i+1].Style = 0
			return nil
		}
	}

	links.Content = append(links.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: target},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: source},
	)

	return nil
}
