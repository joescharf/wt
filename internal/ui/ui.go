package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
)

// UI provides colored output and respects verbose/dry-run modes.
type UI struct {
	Verbose bool
	DryRun  bool
	Out     io.Writer
	ErrOut  io.Writer
}

// New creates a UI with default stdout/stderr writers.
func New() *UI {
	return &UI{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

var (
	infoPrefix    = color.New(color.FgHiBlue).Sprint("i")
	successPrefix = color.New(color.FgHiGreen).Sprint("✓")
	warningPrefix = color.New(color.FgHiYellow).Sprint("⚠")
	errorPrefix   = color.New(color.FgHiRed).Sprint("✗")
	verbosePrefix = color.New(color.FgHiBlue).Sprint("  →")
	cyan          = color.New(color.FgHiCyan).SprintFunc()
	green         = color.New(color.FgHiGreen).SprintFunc()
	yellow        = color.New(color.FgHiYellow).SprintFunc()
	red           = color.New(color.FgHiRed).SprintFunc()
)

// Cyan returns a cyan-colored string for use in messages.
func Cyan(s string) string {
	return cyan(s)
}

// Green returns a green-colored string.
func Green(s string) string {
	return green(s)
}

// Yellow returns a yellow-colored string.
func Yellow(s string) string {
	return yellow(s)
}

// Red returns a red-colored string.
func Red(s string) string {
	return red(s)
}

// StatusColor returns the string colored by status: green for "open", yellow for "stale", red for "closed".
func StatusColor(status string) string {
	switch status {
	case "open":
		return green(status)
	case "stale":
		return yellow(status)
	case "closed":
		return red(status)
	default:
		return status
	}
}

// GitStatusColor returns the string colored by git status.
// "dirty" (with or without +N/-N) is red, "+N"/"-N" without dirty is yellow, "clean" is green.
func GitStatusColor(status string) string {
	switch {
	case strings.HasPrefix(status, "dirty"):
		return red(status)
	case status == "clean":
		return green(status)
	case strings.HasPrefix(status, "+") || strings.HasPrefix(status, "-"):
		return yellow(status)
	default:
		return status
	}
}

func (u *UI) Info(format string, a ...any) {
	fmt.Fprintf(u.Out, "%s %s\n", infoPrefix, fmt.Sprintf(format, a...))
}

func (u *UI) Success(format string, a ...any) {
	fmt.Fprintf(u.Out, "%s %s\n", successPrefix, fmt.Sprintf(format, a...))
}

func (u *UI) Warning(format string, a ...any) {
	fmt.Fprintf(u.ErrOut, "%s %s\n", warningPrefix, fmt.Sprintf(format, a...))
}

func (u *UI) Error(format string, a ...any) {
	fmt.Fprintf(u.ErrOut, "%s %s\n", errorPrefix, fmt.Sprintf(format, a...))
}

func (u *UI) VerboseLog(format string, a ...any) {
	if u.Verbose {
		fmt.Fprintf(u.Out, "%s %s\n", verbosePrefix, fmt.Sprintf(format, a...))
	}
}

func (u *UI) DryRunMsg(format string, a ...any) {
	if u.DryRun {
		u.Warning("[DRY-RUN] "+format, a...)
	}
}
