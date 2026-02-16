package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RotatingWriter is a writer that rotates log files
type RotatingWriter struct {
	filename    string
	maxSize     int64 // bytes
	maxAge      int   // days
	compress    bool
	currentFile *os.File
	currentSize int64
}

// NewRotatingWriter creates a new rotating writer
func NewRotatingWriter(filename string, maxSizeMB int, maxAge int, compress bool) (*RotatingWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open file
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	rw := &RotatingWriter{
		filename:    filename,
		maxSize:     int64(maxSizeMB) * 1024 * 1024,
		maxAge:      maxAge,
		compress:    compress,
		currentFile: file,
		currentSize: info.Size(),
	}

	// Clean up old files
	go rw.cleanup()

	return rw, nil
}

// Write writes data to the log file, rotating if necessary
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	// Check if rotation is needed
	if w.currentSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	// Write to file
	n, err = w.currentFile.Write(p)
	w.currentSize += int64(n)
	return n, err
}

// Close closes the current log file
func (w *RotatingWriter) Close() error {
	if w.currentFile != nil {
		return w.currentFile.Close()
	}
	return nil
}

// rotate rotates the log file
func (w *RotatingWriter) rotate() error {
	// Close current file
	if err := w.currentFile.Close(); err != nil {
		return err
	}

	// Generate rotated filename
	timestamp := time.Now().Format("20060102-150405")
	rotatedName := fmt.Sprintf("%s.%s", w.filename, timestamp)

	// Rename current file
	if err := os.Rename(w.filename, rotatedName); err != nil {
		return err
	}

	// Compress if enabled
	if w.compress {
		go w.compressFile(rotatedName)
	}

	// Open new file
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	w.currentFile = file
	w.currentSize = 0

	return nil
}

// compressFile compresses a log file
func (w *RotatingWriter) compressFile(filename string) error {
	// Open source file
	src, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer src.Close()

	// Create compressed file
	dst, err := os.Create(filename + ".gz")
	if err != nil {
		return err
	}
	defer dst.Close()

	// Create gzip writer
	gzw := gzip.NewWriter(dst)
	defer gzw.Close()

	// Copy data
	if _, err := io.Copy(gzw, src); err != nil {
		return err
	}

	// Remove original file
	return os.Remove(filename)
}

// cleanup removes old log files
func (w *RotatingWriter) cleanup() {
	if w.maxAge <= 0 {
		return
	}

	// Get directory
	dir := filepath.Dir(w.filename)
	base := filepath.Base(w.filename)

	// Find old files
	files, err := filepath.Glob(filepath.Join(dir, base+".*"))
	if err != nil {
		return
	}

	// Sort by modification time
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var infos []fileInfo
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		infos = append(infos, fileInfo{
			path:    file,
			modTime: info.ModTime(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].modTime.Before(infos[j].modTime)
	})

	// Remove old files
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	for _, info := range infos {
		if info.modTime.Before(cutoff) {
			os.Remove(info.path)
			// Also remove .gz file if it exists
			if !strings.HasSuffix(info.path, ".gz") {
				os.Remove(info.path + ".gz")
			}
		}
	}
}
