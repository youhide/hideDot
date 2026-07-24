// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "gopkg.in/yaml.v3"

// LinkStatus represents the state of a symlink
type LinkStatus int

const (
	StatusOK LinkStatus = iota
	StatusMissing
	StatusBroken
	StatusMismatch
	StatusNotSymlink
)

func (s LinkStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusMissing:
		return "MISSING"
	case StatusBroken:
		return "BROKEN"
	case StatusMismatch:
		return "MISMATCH"
	case StatusNotSymlink:
		return "NOT_SYMLINK"
	default:
		return "UNKNOWN"
	}
}

// LinkDefaults holds the link behaviour for a config section. The fields are
// pointers so an omitted key can be told apart from an explicit false — without
// that, writing `defaults.link` without `backup:` would silently disable
// backups while `force: true` overwrites real files.
type LinkDefaults struct {
	Relink           *bool `yaml:"relink,omitempty"`
	Force            *bool `yaml:"force,omitempty"`
	Backup           *bool `yaml:"backup,omitempty"`
	RemoveDuplicates *bool `yaml:"remove_duplicates,omitempty"`
}

// linkOptions is the resolved form of LinkDefaults for one config section.
type linkOptions struct {
	force            bool
	relink           bool
	backup           bool
	removeDuplicates bool
}

// Config represents a single configuration section
type Config struct {
	Defaults *struct {
		Link LinkDefaults `yaml:"link"`
	} `yaml:"defaults,omitempty"`
	Profile string             `yaml:"profile,omitempty"`
	Link    map[string]string  `yaml:"link,omitempty"`
	Create  []string           `yaml:"create,omitempty"`
	Git     map[string]GitRepo `yaml:"git,omitempty"`
	Shell   []ShellCommand     `yaml:"shell,omitempty"`
	Hooks   *Hooks             `yaml:"hooks,omitempty"`
}

// ShellCommand can be either [command, description] or {command, description, stdin}
type ShellCommand struct {
	Command     string
	Description string
	Stdin       string
}

// UnmarshalYAML handles both array and map formats for shell commands
func (s *ShellCommand) UnmarshalYAML(node *yaml.Node) error {
	// Try array format first: [command, description]
	if node.Kind == yaml.SequenceNode {
		var arr []string
		if err := node.Decode(&arr); err != nil {
			return err
		}
		if len(arr) >= 2 {
			s.Command = arr[0]
			s.Description = arr[1]
		}
		return nil
	}

	// Try map format: {command: ..., description: ..., stdin: ...}
	var m struct {
		Command     string `yaml:"command"`
		Description string `yaml:"description"`
		Stdin       string `yaml:"stdin"`
	}
	if err := node.Decode(&m); err != nil {
		return err
	}
	s.Command = m.Command
	s.Description = m.Description
	s.Stdin = m.Stdin
	return nil
}

// Hooks for pre/post operations
type Hooks struct {
	PreLink   []string `yaml:"pre_link,omitempty"`
	PostLink  []string `yaml:"post_link,omitempty"`
	PreShell  []string `yaml:"pre_shell,omitempty"`
	PostShell []string `yaml:"post_shell,omitempty"`
}

// GitRepo represents a git repository configuration
type GitRepo struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// LinkInfo stores detailed information about a link
type LinkInfo struct {
	Target       string
	Source       string
	Status       LinkStatus
	CurrentDest  string
	ErrorMessage string
}

// TemplateData contains variables for template expansion
type TemplateData struct {
	Hostname string
	Username string
	HomeDir  string
	OS       string
	Arch     string
	Date     string
}
