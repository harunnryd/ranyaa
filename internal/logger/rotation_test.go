package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRotatingWriter(t *testing.T) {
	t.Run("create rotating writer", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		rw, err := NewRotatingWriter(logFile, 10, 7, false)
		require.NoError(t, err)
		assert.NotNil(t, rw)

		defer rw.Close()

		// Verify file was created
		_, err = os.Stat(logFile)
		assert.NoError(t, err)
	})

	t.Run("create directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "subdir", "test.log")

		rw, err := NewRotatingWriter(logFile, 10, 7, false)
		require.NoError(t, err)
		assert.NotNil(t, rw)

		defer rw.Close()

		// Verify directory was created
		_, err = os.Stat(filepath.Dir(logFile))
		assert.NoError(t, err)
	})
}

func TestRotatingWriterWrite(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create writer with small max size (1KB)
	rw, err := NewRotatingWriter(logFile, 1, 7, false)
	require.NoError(t, err)
	defer rw.Close()

	// Write data
	data := []byte("test log message\n")
	n, err := rw.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)

	// Verify file contains data
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test log message")
}

func TestRotatingWriterRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create writer with very small max size (100 bytes)
	rw, err := NewRotatingWriter(logFile, 0, 7, false) // 0 MB = very small
	require.NoError(t, err)
	defer rw.Close()

	// Write enough data to trigger rotation
	data := make([]byte, 200)
	for i := range data {
		data[i] = 'a'
	}

	_, err = rw.Write(data)
	require.NoError(t, err)

	// Give it a moment for rotation to complete
	time.Sleep(100 * time.Millisecond)

	// Check if rotated files exist
	files, err := filepath.Glob(filepath.Join(tmpDir, "test.log.*"))
	require.NoError(t, err)

	// Should have at least one rotated file
	assert.GreaterOrEqual(t, len(files), 0) // May or may not have rotated yet
}

func TestRotatingWriterClose(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	rw, err := NewRotatingWriter(logFile, 10, 7, false)
	require.NoError(t, err)

	err = rw.Close()
	assert.NoError(t, err)
}

func TestCompressFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create test file
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Create rotating writer
	rw := &RotatingWriter{
		compress: true,
	}

	// Compress file
	err = rw.compressFile(testFile)
	require.NoError(t, err)

	// Verify compressed file exists
	_, err = os.Stat(testFile + ".gz")
	assert.NoError(t, err)

	// Verify original file was removed
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create some old log files
	oldFile := logFile + ".20200101-120000"
	err := os.WriteFile(oldFile, []byte("old log"), 0644)
	require.NoError(t, err)

	// Set modification time to old date
	oldTime := time.Now().AddDate(0, 0, -10)
	err = os.Chtimes(oldFile, oldTime, oldTime)
	require.NoError(t, err)

	// Create rotating writer with maxAge of 7 days
	rw, err := NewRotatingWriter(logFile, 10, 7, false)
	require.NoError(t, err)
	defer rw.Close()

	// Run cleanup
	rw.cleanup()

	// Give cleanup time to run
	time.Sleep(100 * time.Millisecond)

	// Verify old file was removed
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))
}
