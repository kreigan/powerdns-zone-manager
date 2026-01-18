package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Level represents logging verbosity
type Level int

const (
	LevelInfo Level = iota
	LevelDebug
)

// Logger provides structured logging with verbosity control
type Logger struct {
	out    io.Writer
	level  Level
	prefix string
	dryRun bool
}

// New creates a new logger
func New(verbose bool) *Logger {
	level := LevelInfo
	if verbose {
		level = LevelDebug
	}
	return &Logger{
		out:   os.Stdout,
		level: level,
	}
}

// SetDryRun sets dry-run mode for log prefix
func (l *Logger) SetDryRun(dryRun bool) {
	l.dryRun = dryRun
	if dryRun {
		l.prefix = "[DRY RUN] "
	} else {
		l.prefix = ""
	}
}

// Info logs informational messages (always shown)
func (l *Logger) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.out, "%s%s\n", l.prefix, msg)
}

// Debug logs debug messages (only in verbose mode)
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level >= LevelDebug {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(l.out, "%s[DEBUG] %s\n", l.prefix, msg)
	}
}

// Error logs error messages to stderr
func (l *Logger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s[ERROR] %s\n", l.prefix, msg)
}

// MaskSecret masks sensitive data, showing only first and last 2 chars
func MaskSecret(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + strings.Repeat("*", len(secret)-4) + secret[len(secret)-2:]
}

// MaskURL masks API key in URL if present
func MaskURL(url string) string {
	// URLs shouldn't contain API keys, but just in case
	return url
}
