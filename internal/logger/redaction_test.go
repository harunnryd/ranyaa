package logger

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedactor(t *testing.T) {
	r := NewRedactor()
	assert.NotNil(t, r)
	assert.NotEmpty(t, r.patterns)
}

func TestRedact(t *testing.T) {
	r := NewRedactor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "anthropic API key",
			input:    "API key: sk-ant-api03-test123456789012345678901234567890",
			expected: "API key: [REDACTED]",
		},
		{
			name:     "openai API key",
			input:    "API key: sk-test123456789abcdefghijklmnopqrstuvwxyz",
			expected: "API key: [REDACTED]",
		},
		{
			name:     "bearer token",
			input:    "Authorization: Bearer abc123.def456.ghi789",
			expected: "Authorization: [REDACTED]",
		},
		{
			name:     "telegram bot token",
			input:    "Bot token: 123456789:ABCdefGHIjklMNOpqrsTUVwxyz-1234567",
			expected: "Bot token: [REDACTED]",
		},
		{
			name:     "password",
			input:    `password: "secret123"`,
			expected: `[REDACTED]`,
		},
		{
			name:     "no sensitive data",
			input:    "This is a normal log message",
			expected: "This is a normal log message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input)
			if tt.name == "no sensitive data" {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.Contains(t, result, "[REDACTED]", "should contain [REDACTED] for: %s", tt.input)
			}
		})
	}
}

func TestAddPattern(t *testing.T) {
	r := NewRedactor()

	t.Run("valid pattern", func(t *testing.T) {
		err := r.AddPattern(`custom-[0-9]+`)
		assert.NoError(t, err)

		result := r.Redact("Value: custom-12345")
		assert.Contains(t, result, "[REDACTED]")
	})

	t.Run("invalid pattern", func(t *testing.T) {
		err := r.AddPattern(`[invalid`)
		assert.Error(t, err)
	})
}

func TestWrap(t *testing.T) {
	r := NewRedactor()
	buf := &bytes.Buffer{}

	writer := r.Wrap(buf)
	assert.NotNil(t, writer)

	// Write sensitive data
	n, err := writer.Write([]byte("API key: sk-test123456789abcdefghijklmnopqrstuvwxyz"))
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	// Check that data was redacted
	output := buf.String()
	assert.Contains(t, output, "[REDACTED]")
	assert.NotContains(t, output, "sk-test123456789abcdef")
}

func TestRedactingWriter(t *testing.T) {
	r := NewRedactor()
	buf := &bytes.Buffer{}
	writer := &redactingWriter{
		writer:   buf,
		redactor: r,
	}

	t.Run("write with sensitive data", func(t *testing.T) {
		buf.Reset()

		data := []byte("Token: sk-ant-api03-test123456789012345678901234567890")
		n, err := writer.Write(data)

		require.NoError(t, err)
		assert.Greater(t, n, 0)

		output := buf.String()
		assert.Contains(t, output, "[REDACTED]")
	})

	t.Run("write without sensitive data", func(t *testing.T) {
		buf.Reset()

		data := []byte("Normal log message")
		n, err := writer.Write(data)

		require.NoError(t, err)
		assert.Greater(t, n, 0)

		output := buf.String()
		assert.Equal(t, "Normal log message", output)
	})
}
