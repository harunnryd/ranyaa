package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog.Logger with additional functionality
type Logger struct {
	logger   zerolog.Logger
	file     *os.File
	redactor *Redactor
}

// Config holds logger configuration
type Config struct {
	Level     string // debug, info, warn, error
	File      string // log file path
	Console   bool   // enable console output
	Pretty    bool   // pretty format for console
	Redaction bool   // enable sensitive data redaction
	MaxSize   int    // max size in MB before rotation
	MaxAge    int    // max age in days
	Compress  bool   // compress rotated logs
}

// New creates a new logger
func New(cfg Config) (*Logger, error) {
	// Parse log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Create writers
	var writers []io.Writer

	// Console writer
	if cfg.Console {
		var consoleWriter io.Writer = os.Stdout
		if cfg.Pretty {
			consoleWriter = zerolog.ConsoleWriter{
				Out:        os.Stdout,
				TimeFormat: time.RFC3339,
			}
		}
		writers = append(writers, consoleWriter)
	}

	// File writer
	var file *os.File
	if cfg.File != "" {
		// Ensure directory exists
		dir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		// Open log file
		file, err = os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}

		writers = append(writers, file)
	}

	// Create multi-writer
	var writer io.Writer
	if len(writers) == 0 {
		writer = os.Stdout
	} else if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	// Create redactor if enabled
	var redactor *Redactor
	if cfg.Redaction {
		redactor = NewRedactor()
		writer = redactor.Wrap(writer)
	}

	// Create logger
	logger := zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Logger()

	// Set global logger
	log.Logger = logger

	return &Logger{
		logger:   logger,
		file:     file,
		redactor: redactor,
	}, nil
}

// Close closes the logger and any open files
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Debug logs a debug message
func (l *Logger) Debug() *zerolog.Event {
	return l.logger.Debug()
}

// Info logs an info message
func (l *Logger) Info() *zerolog.Event {
	return l.logger.Info()
}

// Warn logs a warning message
func (l *Logger) Warn() *zerolog.Event {
	return l.logger.Warn()
}

// Error logs an error message
func (l *Logger) Error() *zerolog.Event {
	return l.logger.Error()
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal() *zerolog.Event {
	return l.logger.Fatal()
}

// With creates a child logger with additional context
func (l *Logger) With() zerolog.Context {
	return l.logger.With()
}

// GetZerolog returns the underlying zerolog.Logger
func (l *Logger) GetZerolog() zerolog.Logger {
	return l.logger
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:     "info",
		Console:   true,
		Pretty:    true,
		Redaction: true,
		MaxSize:   100,
		MaxAge:    7,
		Compress:  true,
	}
}
