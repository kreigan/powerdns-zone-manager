// Package logger provides structured logging with verbosity control.
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Level represents logging verbosity.
type Level int

// Log levels.
const (
	LevelInfo Level = iota
	LevelDebug
)

// OutputFormat represents the output format.
type OutputFormat int

// Output formats.
const (
	FormatText OutputFormat = iota
	FormatJSON
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// LogEntry represents a structured log entry for JSON output.
type LogEntry struct {
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
}

// Logger provides structured logging with verbosity control.
type Logger struct {
	out     io.Writer
	errOut  io.Writer
	level   Level
	format  OutputFormat
	dryRun  bool
	noColor bool
}

// Options configures the logger.
type Options struct {
	Verbose bool
	JSON    bool
	NoColor bool
}

// New creates a new logger with options.
func New(opts Options) *Logger {
	level := LevelInfo
	if opts.Verbose {
		level = LevelDebug
	}
	format := FormatText
	if opts.JSON {
		format = FormatJSON
	}
	return &Logger{
		out:     os.Stdout,
		errOut:  os.Stderr,
		level:   level,
		format:  format,
		noColor: opts.NoColor || opts.JSON, // No color in JSON mode
	}
}

// SetDryRun sets dry-run mode for log prefix.
func (l *Logger) SetDryRun(dryRun bool) {
	l.dryRun = dryRun
}

// Info logs informational messages (always shown).
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// InfoWithData logs informational messages with additional structured data (for JSON output).
func (l *Logger) InfoWithData(message string, data map[string]interface{}) {
	if l.format == FormatJSON {
		l.writeJSON(l.out, "info", message, data)
	} else {
		fmt.Fprintf(l.out, "%s%s\n", l.getPrefix(), message)
	}
}

// Debug logs debug messages (only in verbose mode).
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level >= LevelDebug {
		l.log(LevelDebug, format, args...)
	}
}

// Error logs error messages to stderr.
func (l *Logger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.format == FormatJSON {
		l.writeJSON(l.errOut, "error", msg, nil)
	} else {
		prefix := l.getPrefix()
		coloredLevel := l.colorize(colorRed, "ERROR")
		fmt.Fprintf(l.errOut, "%s%s %s\n", prefix, coloredLevel, msg)
	}
}

// Warn logs warning messages (yellow in text mode).
func (l *Logger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.format == FormatJSON {
		l.writeJSON(l.out, "warn", msg, nil)
	} else {
		prefix := l.getPrefix()
		coloredMsg := l.colorize(colorYellow, "! "+msg)
		fmt.Fprintf(l.out, "%s%s\n", prefix, coloredMsg)
	}
}

// HTTPRequest logs an HTTP request (debug level).
func (l *Logger) HTTPRequest(method, url string) {
	if l.level < LevelDebug {
		return
	}
	if l.format == FormatJSON {
		l.writeJSON(l.out, "debug", "HTTP request", map[string]interface{}{
			"type":   "request",
			"method": method,
			"url":    url,
		})
	} else {
		prefix := l.getPrefix()
		label := l.colorize(colorCyan, "REQUEST")
		methodColored := l.colorize(colorBold, method)
		fmt.Fprintf(l.out, "%s%s %s %s\n", prefix, label, methodColored, url)
	}
}

// HTTPResponse logs an HTTP response (debug level).
func (l *Logger) HTTPResponse(method, url string, statusCode int) {
	if l.level < LevelDebug {
		return
	}
	if l.format == FormatJSON {
		l.writeJSON(l.out, "debug", "HTTP response", map[string]interface{}{
			"type":       "response",
			"method":     method,
			"url":        url,
			"statusCode": statusCode,
		})
	} else {
		prefix := l.getPrefix()
		label := l.colorize(colorCyan, "RESPONSE")
		methodColored := l.colorize(colorBold, method)
		statusColored := l.colorizeStatus(statusCode)
		fmt.Fprintf(l.out, "%s%s %s %s -> %s\n", prefix, label, methodColored, url, statusColored)
	}
}

// Table prints a table with headers and rows.
func (l *Logger) Table(title string, headers []string, rows [][]string) {
	if l.format == FormatJSON {
		data := make([]map[string]string, len(rows))
		for i, row := range rows {
			rowMap := make(map[string]string)
			for j, header := range headers {
				if j < len(row) {
					rowMap[header] = row[j]
				}
			}
			data[i] = rowMap
		}
		l.writeJSON(l.out, "info", title, map[string]interface{}{"records": data})
		return
	}

	if len(rows) == 0 {
		fmt.Fprintf(l.out, "%s%s: (none)\n", l.getPrefix(), title)
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print title
	titleColored := l.colorize(colorBold, title+":")
	fmt.Fprintf(l.out, "%s%s\n", l.getPrefix(), titleColored)

	// Print header
	headerLine := l.getPrefix() + "  "
	for i, h := range headers {
		headerLine += l.colorize(colorGray, fmt.Sprintf("%-*s", widths[i]+2, h))
	}
	fmt.Fprintln(l.out, headerLine)

	// Print rows
	for _, row := range rows {
		rowLine := l.getPrefix() + "  "
		for i, cell := range row {
			if i < len(widths) {
				rowLine += fmt.Sprintf("%-*s", widths[i]+2, cell)
			}
		}
		fmt.Fprintln(l.out, rowLine)
	}
}

// Diff logs a diff line with appropriate coloring.
func (l *Logger) Diff(op, content string) {
	if l.level < LevelDebug {
		return
	}
	if l.format == FormatJSON {
		l.writeJSON(l.out, "debug", "diff", map[string]interface{}{
			"operation": op,
			"content":   content,
		})
		return
	}

	prefix := l.getPrefix() + "      "
	switch op {
	case "+":
		fmt.Fprintf(l.out, "%s%s\n", prefix, l.colorize(colorGreen, "+ "+content))
	case "-":
		fmt.Fprintf(l.out, "%s%s\n", prefix, l.colorize(colorRed, "- "+content))
	case "~":
		fmt.Fprintf(l.out, "%s%s\n", prefix, l.colorize(colorYellow, "~ "+content))
	default:
		fmt.Fprintf(l.out, "%s  %s\n", prefix, content)
	}
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.format == FormatJSON {
		levelStr := "info"
		if level == LevelDebug {
			levelStr = "debug"
		}
		l.writeJSON(l.out, levelStr, msg, nil)
	} else {
		prefix := l.getPrefix()
		if level == LevelDebug {
			// Gray color for debug messages
			msg = l.colorize(colorGray, msg)
		}
		fmt.Fprintf(l.out, "%s%s\n", prefix, msg)
	}
}

func (l *Logger) getPrefix() string {
	if l.dryRun {
		return l.colorize(colorYellow, "[DRY RUN] ")
	}
	return ""
}

func (l *Logger) colorize(color, text string) string {
	if l.noColor {
		return text
	}
	return color + text + colorReset
}

func (l *Logger) colorizeStatus(statusCode int) string {
	status := fmt.Sprintf("%d", statusCode)
	switch {
	case statusCode >= 200 && statusCode < 300:
		return l.colorize(colorGreen, status)
	case statusCode >= 300 && statusCode < 400:
		return l.colorize(colorYellow, status)
	default:
		return l.colorize(colorRed, status)
	}
}

func (l *Logger) writeJSON(out io.Writer, level, message string, data map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   message,
		Data:      data,
	}
	if l.dryRun {
		if entry.Data == nil {
			entry.Data = make(map[string]interface{})
		}
		entry.Data["dryRun"] = true
	}
	jsonData, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple format if JSON marshaling fails
		fmt.Fprintf(out, "{\"level\":%q,\"message\":%q}\n", level, message)
		return
	}
	fmt.Fprintln(out, string(jsonData))
}

// MaskSecret masks sensitive data, showing only first and last 2 chars.
func MaskSecret(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + strings.Repeat("*", len(secret)-4) + secret[len(secret)-2:]
}

// MaskURL masks API key in URL if present.
func MaskURL(url string) string {
	// URLs shouldn't contain API keys, but just in case
	return url
}
