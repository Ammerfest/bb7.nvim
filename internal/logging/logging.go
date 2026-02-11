package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger handles debug logging to file and stderr.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	enabled bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Get returns the default logger instance.
func Get() *Logger {
	once.Do(func() {
		defaultLogger = &Logger{}
		defaultLogger.init()
	})
	return defaultLogger
}

func (l *Logger) init() {
	// Check if debug mode is enabled via env var or config file
	debugEnv := os.Getenv("BB7_DEBUG")

	// Also check for ~/.bb7/debug file
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "BB-7 log: failed to get home dir: %v\n", err)
		return
	}

	debugFile := filepath.Join(home, ".bb7", "debug")
	_, debugFileErr := os.Stat(debugFile)
	debugFileExists := debugFileErr == nil

	if debugEnv != "1" && !debugFileExists {
		l.enabled = false
		return
	}

	l.enabled = true

	// Create logs directory
	if err != nil {
		fmt.Fprintf(os.Stderr, "BB-7 log: failed to get home dir: %v\n", err)
		return
	}

	logsDir := filepath.Join(home, ".bb7", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "BB-7 log: failed to create logs dir %s: %v\n", logsDir, err)
		return
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logPath := filepath.Join(logsDir, fmt.Sprintf("bb7-%s.log", timestamp))

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "BB-7 log: failed to open log file %s: %v\n", logPath, err)
		return
	}

	l.file = file

	// Log how debugging was enabled
	if debugEnv == "1" {
		l.logf("INFO", "Logging started (BB7_DEBUG=1)")
	} else {
		l.logf("INFO", "Logging started (~/.bb7/debug exists)")
	}
	l.logf("INFO", "Log file: %s", logPath)
}

// Enabled returns whether debug logging is enabled.
func (l *Logger) Enabled() bool {
	return l.enabled
}

func (l *Logger) logf(level, format string, args ...any) {
	if l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.file, "[%s] %s [backend]: %s\n", timestamp, level, msg)
}

// Debug logs a debug message (file only).
func (l *Logger) Debug(format string, args ...any) {
	if !l.enabled {
		return
	}
	l.logf("DEBUG", format, args...)
}

// Info logs an info message (file only).
func (l *Logger) Info(format string, args ...any) {
	if !l.enabled {
		return
	}
	l.logf("INFO", format, args...)
}

// Error logs an error message (file and stderr).
func (l *Logger) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "BB-7 error: %s\n", msg)
	if l.enabled {
		l.logf("ERROR", format, args...)
	}
}

// Request logs an incoming request.
func (l *Logger) Request(action string, raw string) {
	if !l.enabled {
		return
	}
	l.logf("REQ", "[%s] %s", action, truncate(raw, 500))
}

// Response logs an outgoing response.
func (l *Logger) Response(msgType string, raw string) {
	if !l.enabled {
		return
	}
	l.logf("RESP", "[%s] %s", msgType, truncate(raw, 500))
}

// Stream logs a streaming event.
func (l *Logger) Stream(eventType string, content string) {
	if !l.enabled {
		return
	}
	l.logf("STREAM", "[%s] %s", eventType, truncate(content, 200))
}

// ToolCall logs a tool call.
func (l *Logger) ToolCall(name string, args string) {
	if !l.enabled {
		return
	}
	l.logf("TOOL", "[%s] %s", name, truncate(args, 500))
}

// Close closes the log file.
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// Writer returns an io.Writer for the log file (for external use).
func (l *Logger) Writer() io.Writer {
	if l.file != nil {
		return l.file
	}
	return io.Discard
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
