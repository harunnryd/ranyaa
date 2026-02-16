package logger

import (
	"io"
	"regexp"
)

// Redactor redacts sensitive information from logs
type Redactor struct {
	patterns []*regexp.Regexp
}

// NewRedactor creates a new redactor with default patterns
func NewRedactor() *Redactor {
	return &Redactor{
		patterns: []*regexp.Regexp{
			// API keys
			regexp.MustCompile(`sk-[a-zA-Z0-9_-]{20,}`),
			regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]{20,}`),

			// Bearer tokens
			regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._-]+`),

			// Telegram bot tokens
			regexp.MustCompile(`\d{8,10}:[a-zA-Z0-9_-]{30,}`),

			// Passwords
			regexp.MustCompile(`password["\s:=]+[^\s"]+`),
			regexp.MustCompile(`pwd["\s:=]+[^\s"]+`),

			// Auth tokens
			regexp.MustCompile(`token["\s:=]+[a-zA-Z0-9._-]{20,}`),

			// AWS keys
			regexp.MustCompile(`AKIA[0-9A-Z]{16}`),

			// Generic secrets
			regexp.MustCompile(`secret["\s:=]+[^\s"]+`),
		},
	}
}

// AddPattern adds a custom redaction pattern
func (r *Redactor) AddPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	r.patterns = append(r.patterns, re)
	return nil
}

// Redact redacts sensitive information from a string
func (r *Redactor) Redact(s string) string {
	result := s
	for _, pattern := range r.patterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

// Wrap wraps an io.Writer to redact sensitive information
func (r *Redactor) Wrap(w io.Writer) io.Writer {
	return &redactingWriter{
		writer:   w,
		redactor: r,
	}
}

// redactingWriter is an io.Writer that redacts sensitive information
type redactingWriter struct {
	writer   io.Writer
	redactor *Redactor
}

func (w *redactingWriter) Write(p []byte) (n int, err error) {
	redacted := w.redactor.Redact(string(p))
	return w.writer.Write([]byte(redacted))
}
