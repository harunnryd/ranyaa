package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// NodeStorage handles persistence of node data
type NodeStorage struct {
	storagePath      string
	autoSaveInterval time.Duration
	autoSaveEnabled  bool
	autoSaveTicker   *time.Ticker
	autoSaveStop     chan struct{}
	mu               sync.RWMutex
	getNodesFunc     func() []*Node
}

// storageData represents the data structure for persistence
type storageData struct {
	Nodes       []*Node `json:"nodes"`
	DefaultNode string  `json:"defaultNode"`
	Version     string  `json:"version"`
	SavedAt     int64   `json:"savedAt"`
}

// NewNodeStorage creates a new NodeStorage
func NewNodeStorage(storagePath string, autoSaveInterval time.Duration) *NodeStorage {
	return &NodeStorage{
		storagePath:      storagePath,
		autoSaveInterval: autoSaveInterval,
		autoSaveStop:     make(chan struct{}),
	}
}

// LoadNodes loads nodes from storage
func (ns *NodeStorage) LoadNodes() ([]*Node, string, error) {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(ns.storagePath); os.IsNotExist(err) {
		log.Info().Str("path", ns.storagePath).Msg("Storage file does not exist, starting fresh")
		return []*Node{}, "", nil
	}

	// Read file
	data, err := os.ReadFile(ns.storagePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read storage file: %w", err)
	}

	// Parse JSON
	var storage storageData
	if err := json.Unmarshal(data, &storage); err != nil {
		return nil, "", fmt.Errorf("failed to parse storage file: %w", err)
	}

	log.Info().
		Str("path", ns.storagePath).
		Int("nodeCount", len(storage.Nodes)).
		Msg("Nodes loaded from storage")

	return storage.Nodes, storage.DefaultNode, nil
}

// SaveNodes saves nodes to storage
func (ns *NodeStorage) SaveNodes(nodes []*Node, defaultNode string) error {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	// Create storage data
	storage := storageData{
		Nodes:       nodes,
		DefaultNode: defaultNode,
		Version:     "1.0",
		SavedAt:     time.Now().Unix(),
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal nodes: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(ns.storagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Write to temporary file first
	tempPath := ns.storagePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, ns.storagePath); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	log.Debug().
		Str("path", ns.storagePath).
		Int("nodeCount", len(nodes)).
		Msg("Nodes saved to storage")

	return nil
}

// StartAutoSave starts automatic periodic saving
func (ns *NodeStorage) StartAutoSave(getNodesFunc func() []*Node, getDefaultNodeFunc func() string) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if ns.autoSaveEnabled {
		return
	}

	ns.getNodesFunc = getNodesFunc
	ns.autoSaveEnabled = true
	ns.autoSaveTicker = time.NewTicker(ns.autoSaveInterval)

	go ns.autoSaveLoop(getDefaultNodeFunc)

	log.Info().
		Dur("interval", ns.autoSaveInterval).
		Msg("Auto-save started")
}

// StopAutoSave stops automatic periodic saving
func (ns *NodeStorage) StopAutoSave() {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if !ns.autoSaveEnabled {
		return
	}

	ns.autoSaveEnabled = false
	if ns.autoSaveTicker != nil {
		ns.autoSaveTicker.Stop()
	}
	close(ns.autoSaveStop)

	log.Info().Msg("Auto-save stopped")
}

// autoSaveLoop is the auto-save loop
func (ns *NodeStorage) autoSaveLoop(getDefaultNodeFunc func() string) {
	for {
		select {
		case <-ns.autoSaveTicker.C:
			nodes := ns.getNodesFunc()
			defaultNode := getDefaultNodeFunc()

			if err := ns.SaveNodes(nodes, defaultNode); err != nil {
				log.Error().Err(err).Msg("Auto-save failed")
			}

		case <-ns.autoSaveStop:
			return
		}
	}
}
