package pairing

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultPendingLimit = 3
	DefaultPendingTTL   = time.Hour
	CodeLength          = 8
)

var (
	ErrPendingLimitReached = errors.New("pairing pending limit reached")
	ErrRequestNotFound     = errors.New("pairing request not found")
	ErrAlreadyAllowlisted  = errors.New("peer is already allowlisted")
)

var codeAlphabet = []rune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

// PendingRequest represents a pending pairing request for a peer.
type PendingRequest struct {
	Channel     string    `json:"channel"`
	PeerID      string    `json:"peer_id"`
	Code        string    `json:"code"`
	RequestedAt time.Time `json:"requested_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// AllowlistEntry represents an approved peer.
type AllowlistEntry struct {
	PeerID  string    `json:"peer_id"`
	AddedAt time.Time `json:"added_at"`
	Reason  string    `json:"reason,omitempty"`
}

// ManagerOptions configures a pairing manager.
type ManagerOptions struct {
	Channel            string
	PendingPath        string
	AllowlistPath      string
	MaxPending         int
	PendingTTL         time.Duration
	BootstrapAllowlist []string
	Now                func() time.Time
}

// Manager manages pairing requests and approved allowlists.
type Manager struct {
	mu sync.Mutex

	channel       string
	pendingPath   string
	allowlistPath string
	maxPending    int
	pendingTTL    time.Duration
	now           func() time.Time

	pending       map[string]PendingRequest
	pendingByCode map[string]string
	allowlist     map[string]AllowlistEntry
	bootstrap     map[string]bool

	pendingModTime   time.Time
	allowlistModTime time.Time
}

type pendingStore struct {
	Requests []PendingRequest `json:"requests"`
}

type allowlistStore struct {
	Entries []AllowlistEntry `json:"entries"`
}

// NewManager creates a new pairing manager.
func NewManager(opts ManagerOptions) (*Manager, error) {
	if strings.TrimSpace(opts.Channel) == "" {
		return nil, fmt.Errorf("pairing channel is required")
	}
	pendingTTL := opts.PendingTTL
	if pendingTTL <= 0 {
		pendingTTL = DefaultPendingTTL
	}
	maxPending := opts.MaxPending
	if maxPending <= 0 {
		maxPending = DefaultPendingLimit
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	manager := &Manager{
		channel:       opts.Channel,
		pendingPath:   strings.TrimSpace(opts.PendingPath),
		allowlistPath: strings.TrimSpace(opts.AllowlistPath),
		maxPending:    maxPending,
		pendingTTL:    pendingTTL,
		now:           nowFn,
		pending:       make(map[string]PendingRequest),
		pendingByCode: make(map[string]string),
		allowlist:     make(map[string]AllowlistEntry),
		bootstrap:     make(map[string]bool),
	}

	for _, peerID := range opts.BootstrapAllowlist {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		manager.bootstrap[peerID] = true
	}

	if err := manager.loadAllowlist(); err != nil {
		return nil, err
	}
	if err := manager.loadPending(); err != nil {
		return nil, err
	}

	return manager, nil
}

// Channel returns the channel associated with the manager.
func (m *Manager) Channel() string {
	return m.channel
}

// IsAllowed returns true if the peer is allowlisted.
func (m *Manager) IsAllowed(peerID string) bool {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshFromDiskLocked()
	if m.bootstrap[peerID] {
		return true
	}
	_, ok := m.allowlist[peerID]
	return ok
}

// ListAllowlist returns allowlisted peers.
func (m *Manager) ListAllowlist() []AllowlistEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshFromDiskLocked()
	entries := make([]AllowlistEntry, 0, len(m.allowlist))
	for _, entry := range m.allowlist {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AddedAt.Before(entries[j].AddedAt)
	})
	return entries
}

// ListPending returns active pending pairing requests.
func (m *Manager) ListPending() []PendingRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshFromDiskLocked()
	m.cleanupExpiredLocked()
	requests := make([]PendingRequest, 0, len(m.pending))
	for _, req := range m.pending {
		requests = append(requests, req)
	}
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].RequestedAt.Before(requests[j].RequestedAt)
	})
	return requests
}

// EnsurePending returns a pending request for the peer, creating one if needed.
// The returned boolean indicates whether a new request was created.
func (m *Manager) EnsurePending(peerID string) (PendingRequest, bool, error) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return PendingRequest{}, false, fmt.Errorf("peer id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshFromDiskLocked()
	m.cleanupExpiredLocked()

	if m.bootstrap[peerID] {
		return PendingRequest{}, false, ErrAlreadyAllowlisted
	}
	if _, ok := m.allowlist[peerID]; ok {
		return PendingRequest{}, false, ErrAlreadyAllowlisted
	}
	if existing, ok := m.pending[peerID]; ok {
		return existing, false, nil
	}
	if len(m.pending) >= m.maxPending {
		return PendingRequest{}, false, ErrPendingLimitReached
	}

	code, err := m.generateUniqueCodeLocked()
	if err != nil {
		return PendingRequest{}, false, err
	}
	now := m.now()
	request := PendingRequest{
		Channel:     m.channel,
		PeerID:      peerID,
		Code:        code,
		RequestedAt: now,
		ExpiresAt:   now.Add(m.pendingTTL),
	}
	m.pending[peerID] = request
	m.pendingByCode[strings.ToUpper(code)] = peerID
	if err := m.savePendingLocked(); err != nil {
		return PendingRequest{}, false, err
	}
	return request, true, nil
}

// Approve approves a pending pairing request by code.
func (m *Manager) Approve(code string) (PendingRequest, error) {
	return m.resolve(code, true)
}

// Reject rejects a pending pairing request by code.
func (m *Manager) Reject(code string) (PendingRequest, error) {
	return m.resolve(code, false)
}

func (m *Manager) resolve(code string, approve bool) (PendingRequest, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return PendingRequest{}, fmt.Errorf("code is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshFromDiskLocked()
	m.cleanupExpiredLocked()

	peerID, ok := m.pendingByCode[code]
	if !ok {
		return PendingRequest{}, ErrRequestNotFound
	}

	request := m.pending[peerID]
	delete(m.pending, peerID)
	delete(m.pendingByCode, code)

	if approve {
		m.allowlist[peerID] = AllowlistEntry{
			PeerID:  peerID,
			AddedAt: m.now(),
			Reason:  fmt.Sprintf("approved via code %s", code),
		}
		if err := m.saveAllowlistLocked(); err != nil {
			return PendingRequest{}, err
		}
	}

	if err := m.savePendingLocked(); err != nil {
		return PendingRequest{}, err
	}

	return request, nil
}

func (m *Manager) cleanupExpiredLocked() {
	now := m.now()
	changed := false
	for peerID, req := range m.pending {
		if now.After(req.ExpiresAt) {
			delete(m.pending, peerID)
			delete(m.pendingByCode, strings.ToUpper(req.Code))
			changed = true
		}
	}
	if changed {
		_ = m.savePendingLocked()
	}
}

func (m *Manager) refreshFromDiskLocked() {
	if m.pendingPath != "" {
		if info, err := os.Stat(m.pendingPath); err == nil {
			if info.ModTime().After(m.pendingModTime) {
				_ = m.loadPending()
			}
		}
	}
	if m.allowlistPath != "" {
		if info, err := os.Stat(m.allowlistPath); err == nil {
			if info.ModTime().After(m.allowlistModTime) {
				_ = m.loadAllowlist()
			}
		}
	}
}

func (m *Manager) generateUniqueCodeLocked() (string, error) {
	for i := 0; i < 5; i++ {
		code, err := generateCode()
		if err != nil {
			return "", err
		}
		if _, exists := m.pendingByCode[strings.ToUpper(code)]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique pairing code")
}

func generateCode() (string, error) {
	buf := make([]byte, CodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate pairing code: %w", err)
	}
	out := make([]rune, CodeLength)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out), nil
}

func (m *Manager) loadPending() error {
	m.pending = make(map[string]PendingRequest)
	m.pendingByCode = make(map[string]string)
	if m.pendingPath == "" {
		return nil
	}
	info, err := os.Stat(m.pendingPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat pending pairing file: %w", err)
	}
	data, err := os.ReadFile(m.pendingPath)
	if err != nil {
		return fmt.Errorf("failed to read pending pairing file: %w", err)
	}
	var store pendingStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("failed to parse pending pairing file: %w", err)
	}
	for _, req := range store.Requests {
		peerID := strings.TrimSpace(req.PeerID)
		code := strings.ToUpper(strings.TrimSpace(req.Code))
		if peerID == "" || code == "" {
			continue
		}
		m.pending[peerID] = req
		m.pendingByCode[code] = peerID
	}
	m.cleanupExpiredLocked()
	m.pendingModTime = info.ModTime()
	return nil
}

func (m *Manager) loadAllowlist() error {
	m.allowlist = make(map[string]AllowlistEntry)
	if m.allowlistPath == "" {
		return nil
	}
	info, err := os.Stat(m.allowlistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat allowlist file: %w", err)
	}
	data, err := os.ReadFile(m.allowlistPath)
	if err != nil {
		return fmt.Errorf("failed to read allowlist file: %w", err)
	}
	var store allowlistStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("failed to parse allowlist file: %w", err)
	}
	for _, entry := range store.Entries {
		peerID := strings.TrimSpace(entry.PeerID)
		if peerID == "" {
			continue
		}
		m.allowlist[peerID] = entry
	}
	m.allowlistModTime = info.ModTime()
	return nil
}

func (m *Manager) savePendingLocked() error {
	if m.pendingPath == "" {
		return nil
	}
	store := pendingStore{Requests: make([]PendingRequest, 0, len(m.pending))}
	for _, req := range m.pending {
		store.Requests = append(store.Requests, req)
	}
	sort.Slice(store.Requests, func(i, j int) bool {
		return store.Requests[i].RequestedAt.Before(store.Requests[j].RequestedAt)
	})
	if err := writeJSONFile(m.pendingPath, store); err != nil {
		return err
	}
	m.pendingModTime = m.now()
	return nil
}

func (m *Manager) saveAllowlistLocked() error {
	if m.allowlistPath == "" {
		return nil
	}
	store := allowlistStore{Entries: make([]AllowlistEntry, 0, len(m.allowlist))}
	for _, entry := range m.allowlist {
		store.Entries = append(store.Entries, entry)
	}
	sort.Slice(store.Entries, func(i, j int) bool {
		return store.Entries[i].AddedAt.Before(store.Entries[j].AddedAt)
	})
	if err := writeJSONFile(m.allowlistPath, store); err != nil {
		return err
	}
	m.allowlistModTime = m.now()
	return nil
}

func writeJSONFile(path string, payload interface{}) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DefaultPaths returns default pending/allowlist file paths for a channel.
func DefaultPaths(dataDir, channel string) (string, string) {
	base := filepath.Join(strings.TrimSpace(dataDir), "pairing")
	return filepath.Join(base, fmt.Sprintf("%s-pending.json", channel)), filepath.Join(base, fmt.Sprintf("%s-allowlist.json", channel))
}
