package memory

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// FileWatcher watches for file system changes
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	logger   zerolog.Logger
	onDirty  func()
	debounce time.Duration
	timer    *time.Timer
	stopCh   chan struct{}
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(logger zerolog.Logger, onDirty func()) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:  watcher,
		logger:   logger,
		onDirty:  onDirty,
		debounce: 500 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	go fw.run()

	return fw, nil
}

// Watch starts watching a directory
func (fw *FileWatcher) Watch(path string) error {
	return fw.watcher.Add(path)
}

// Stop stops the file watcher
func (fw *FileWatcher) Stop() error {
	close(fw.stopCh)
	return fw.watcher.Close()
}

// run processes file system events
func (fw *FileWatcher) run() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Only watch markdown files
			if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
				continue
			}

			// Handle events
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				fw.logger.Debug().
					Str("file", filepath.Base(event.Name)).
					Str("op", event.Op.String()).
					Msg("File change detected")

				fw.scheduleMarkDirty()
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.Error().Err(err).Msg("File watcher error")

		case <-fw.stopCh:
			return
		}
	}
}

// scheduleMarkDirty debounces the mark dirty operation
func (fw *FileWatcher) scheduleMarkDirty() {
	if fw.timer != nil {
		fw.timer.Stop()
	}

	fw.timer = time.AfterFunc(fw.debounce, func() {
		fw.logger.Debug().Msg("Marking index as dirty after file changes")
		fw.onDirty()
	})
}
