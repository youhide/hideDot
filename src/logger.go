// hideDot - A dotfiles manager
// Copyright (C) 2024-2026 youhide
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "fmt"

// ANSI color codes
const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Red        = "\033[31m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Magenta    = "\033[35m"
	Cyan       = "\033[36m"
	White      = "\033[37m"
	BoldRed    = "\033[1;31m"
	BoldGreen  = "\033[1;32m"
	BoldYellow = "\033[1;33m"
	BoldBlue   = "\033[1;34m"
	BoldCyan   = "\033[1;36m"
)

// Logger handles logging with dry run and color support
type Logger struct {
	dryRun       bool
	useColors    bool
	verbose      bool
	quiet        bool
	errorCount   int
	successCount int
	warnCount    int
}

func (l *Logger) log(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	var prefix string

	if l.dryRun {
		if l.useColors {
			prefix = BoldYellow + "[DRY RUN]" + Reset + " " + BoldCyan + "==>" + Reset
		} else {
			prefix = "[DRY RUN] ==>"
		}
	} else {
		if l.useColors {
			prefix = BoldCyan + "==>" + Reset
		} else {
			prefix = "==>"
		}
	}

	fmt.Printf(prefix+" "+format+"\n", args...)
}

func (l *Logger) success(format string, args ...interface{}) {
	l.successCount++
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Green+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) info(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Blue+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) debug(format string, args ...interface{}) {
	if !l.verbose || l.quiet {
		return
	}
	if l.useColors {
		l.log(Magenta+"[DEBUG] "+format+Reset, args...)
	} else {
		l.log("[DEBUG] "+format, args...)
	}
}

func (l *Logger) warn(format string, args ...interface{}) {
	l.warnCount++
	if l.quiet {
		return
	}
	if l.useColors {
		l.log(Yellow+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) error(format string, args ...interface{}) {
	l.errorCount++
	if l.useColors {
		l.log(Red+format+Reset, args...)
	} else {
		l.log(format, args...)
	}
}

func (l *Logger) heading(format string, args ...interface{}) {
	if l.quiet {
		return
	}
	if l.useColors {
		fmt.Printf("\n"+BoldCyan+format+Reset+"\n", args...)
	} else {
		fmt.Printf("\n"+format+"\n", args...)
	}
}

func (l *Logger) summary() {
	if l.quiet {
		return
	}
	if l.useColors {
		fmt.Printf("\n"+BoldGreen+"%d successful"+Reset+", "+BoldYellow+"%d warnings"+Reset+", "+BoldRed+"%d errors"+Reset+"\n",
			l.successCount, l.warnCount, l.errorCount)
	} else {
		fmt.Printf("\n%d successful, %d warnings, %d errors\n",
			l.successCount, l.warnCount, l.errorCount)
	}
}

func (l *Logger) execute(action func() error) error {
	if l.dryRun {
		return nil
	}
	return action()
}
