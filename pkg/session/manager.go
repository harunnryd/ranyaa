package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Message represents a single conversation turn
type Message struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SessionEntry represents a message with its session key
type SessionEntry struct {
	SessionKey string  `json:"sessionKey"`
	Message    Message `json:"message"`
}

// SessionManager manages conversation persistence using JSONL format
type SessionManager struct {
	sessionsDir string
	writeLocks  map[string]*sync.Mutex
	locksMu     sync.RWMutex
}

// New creates a new SessionManager
func New(sessionsDir string) (*SessionManager, error) {
	observability.EnsureRegistered()

	if sessionsDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		sessionsDir = filepath.Join(homeDir, ".ranya", "sessions")
	}

	// Create sessions directory if it doesn't exist
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	sm := &SessionManager{
		sessionsDir: sessionsDir,
		writeLocks:  make(map[string]*sync.Mutex),
	}

	log.Info().Str("dir", sessionsDir).Msg("Session manager initialized")
	sm.updateActiveSessionsMetric()

	return sm, nil
}

// validateSessionKey validates the session key for security
func (sm *SessionManager) validateSessionKey(sessionKey string) error {
	if sessionKey == "" {
		return fmt.Errorf("session key cannot be empty")
	}
	if strings.Contains(sessionKey, "..") {
		return fmt.Errorf("session key cannot contain '..'")
	}
	if strings.ContainsAny(sessionKey, "/\\") {
		return fmt.Errorf("session key cannot contain path separators")
	}
	if strings.Contains(sessionKey, "\x00") {
		return fmt.Errorf("session key cannot contain null bytes")
	}
	return nil
}

// getSessionPath returns the file path for a session
func (sm *SessionManager) getSessionPath(sessionKey string) string {
	return filepath.Join(sm.sessionsDir, sessionKey+".jsonl")
}

func (sm *SessionManager) updateActiveSessionsMetric() {
	sessions, err := sm.ListSessions()
	if err != nil {
		return
	}
	observability.SetActiveSessions(len(sessions))
}

// getWriteLock gets or creates a write lock for a session
func (sm *SessionManager) getWriteLock(sessionKey string) *sync.Mutex {
	sm.locksMu.Lock()
	defer sm.locksMu.Unlock()

	if lock, exists := sm.writeLocks[sessionKey]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	sm.writeLocks[sessionKey] = lock
	return lock
}

// releaseWriteLock releases a write lock for a session
func (sm *SessionManager) releaseWriteLock(sessionKey string) {
	sm.locksMu.Lock()
	defer sm.locksMu.Unlock()
	delete(sm.writeLocks, sessionKey)
}

// CreateSession creates a new session file
func (sm *SessionManager) CreateSession(sessionKey string) error {
	return sm.CreateSessionWithContext(context.Background(), sessionKey)
}

// CreateSessionWithContext creates a new session file with tracing context.
func (sm *SessionManager) CreateSessionWithContext(ctx context.Context, sessionKey string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.session",
		"session.create",
		attribute.String("session_key", sessionKey),
	)
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, log.Logger).With().Str("session_key", sessionKey).Logger()

	if err := sm.validateSessionKey(sessionKey); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	sessionPath := sm.getSessionPath(sessionKey)

	// Check if session already exists
	if _, err := os.Stat(sessionPath); err == nil {
		logger.Debug().Str("sessionKey", sessionKey).Msg("Session already exists")
		return nil
	}

	// Create empty file with restricted permissions
	file, err := os.OpenFile(sessionPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to create session file: %w", err)
	}
	file.Close()

	sm.updateActiveSessionsMetric()
	logger.Info().Str("sessionKey", sessionKey).Msg("Session created")

	return nil
}

// AppendMessage appends a message to a session
func (sm *SessionManager) AppendMessage(sessionKey string, message Message) error {
	return sm.AppendMessageWithContext(context.Background(), sessionKey, message)
}

// AppendMessageWithContext appends a message to a session with tracing context.
func (sm *SessionManager) AppendMessageWithContext(ctx context.Context, sessionKey string, message Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.session",
		"session.append_message",
		attribute.String("session_key", sessionKey),
		attribute.String("role", message.Role),
	)
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, log.Logger).With().Str("session_key", sessionKey).Logger()
	start := time.Now()
	defer func() {
		observability.RecordSessionSave(time.Since(start))
	}()

	if err := sm.validateSessionKey(sessionKey); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Validate message
	if message.Role == "" {
		return fmt.Errorf("message role cannot be empty")
	}
	if message.Content == "" {
		return fmt.Errorf("message content cannot be empty")
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = time.Now()
	}

	// Get write lock for this session
	lock := sm.getWriteLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	sessionPath := sm.getSessionPath(sessionKey)

	// Create session if it doesn't exist
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		if err := sm.CreateSessionWithContext(ctx, sessionKey); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	// Open file for appending
	file, err := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	// Create session entry
	entry := SessionEntry{
		SessionKey: sessionKey,
		Message:    message,
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write JSON line
	if _, err := file.Write(append(data, '\n')); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Sync to disk
	if err := file.Sync(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to sync file: %w", err)
	}

	logger.Debug().
		Str("sessionKey", sessionKey).
		Str("role", message.Role).
		Msg("Message appended")

	return nil
}

// LoadSession loads all messages from a session
func (sm *SessionManager) LoadSession(sessionKey string) ([]SessionEntry, error) {
	return sm.LoadSessionWithContext(context.Background(), sessionKey)
}

// LoadSessionWithContext loads all messages from a session with tracing context.
func (sm *SessionManager) LoadSessionWithContext(ctx context.Context, sessionKey string) ([]SessionEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.session",
		"session.load",
		attribute.String("session_key", sessionKey),
	)
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, log.Logger).With().Str("session_key", sessionKey).Logger()
	start := time.Now()
	defer func() {
		observability.RecordSessionLoad(time.Since(start))
	}()

	if err := sm.validateSessionKey(sessionKey); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	sessionPath := sm.getSessionPath(sessionKey)

	// Check if session exists
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		logger.Debug().Str("sessionKey", sessionKey).Msg("Session does not exist")
		return []SessionEntry{}, nil
	}

	// Open file for reading
	file, err := os.Open(sessionPath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	var entries []SessionEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if line == "" {
			continue
		}

		var entry SessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			logger.Warn().
				Str("sessionKey", sessionKey).
				Int("line", lineNum).
				Err(err).
				Msg("Failed to parse line, skipping")
			continue
		}

		// Validate entry
		if entry.Message.Role == "" || entry.Message.Content == "" {
			logger.Warn().
				Str("sessionKey", sessionKey).
				Int("line", lineNum).
				Msg("Invalid entry, skipping")
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	logger.Debug().
		Str("sessionKey", sessionKey).
		Int("messages", len(entries)).
		Msg("Session loaded")

	return entries, nil
}

// DeleteSession deletes a session file
func (sm *SessionManager) DeleteSession(sessionKey string) error {
	return sm.DeleteSessionWithContext(context.Background(), sessionKey)
}

// DeleteSessionWithContext deletes a session file with tracing context.
func (sm *SessionManager) DeleteSessionWithContext(ctx context.Context, sessionKey string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = tracing.WithSessionKey(ctx, sessionKey)
	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.session",
		"session.delete",
		attribute.String("session_key", sessionKey),
	)
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, log.Logger).With().Str("session_key", sessionKey).Logger()

	if err := sm.validateSessionKey(sessionKey); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Wait for any in-progress writes
	lock := sm.getWriteLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	sessionPath := sm.getSessionPath(sessionKey)

	// Delete file
	if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	// Release lock
	sm.releaseWriteLock(sessionKey)
	sm.updateActiveSessionsMetric()

	logger.Info().Str("sessionKey", sessionKey).Msg("Session deleted")

	return nil
}

// ListSessions lists all available sessions
func (sm *SessionManager) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(sm.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		sessionKey := strings.TrimSuffix(name, ".jsonl")
		sessions = append(sessions, sessionKey)
	}

	return sessions, nil
}

// RepairSession repairs a corrupted session file
func (sm *SessionManager) RepairSession(sessionKey string) error {
	if err := sm.validateSessionKey(sessionKey); err != nil {
		return err
	}

	// Load session (this will skip corrupted lines)
	entries, err := sm.LoadSession(sessionKey)
	if err != nil {
		return err
	}

	// Get write lock
	lock := sm.getWriteLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	sessionPath := sm.getSessionPath(sessionKey)
	tempPath := sessionPath + ".tmp"

	// Write to temp file
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("failed to marshal entry: %w", err)
		}

		if _, err := file.Write(append(data, '\n')); err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	file.Close()

	// Atomic replace
	if err := os.Rename(tempPath, sessionPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace session file: %w", err)
	}

	log.Info().
		Str("sessionKey", sessionKey).
		Int("entries", len(entries)).
		Msg("Session repaired")

	return nil
}

// GetSessionInfo returns metadata about a session
func (sm *SessionManager) GetSessionInfo(sessionKey string) (map[string]interface{}, error) {
	if err := sm.validateSessionKey(sessionKey); err != nil {
		return nil, err
	}

	sessionPath := sm.getSessionPath(sessionKey)

	info, err := os.Stat(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session does not exist")
		}
		return nil, fmt.Errorf("failed to stat session file: %w", err)
	}

	// Count messages
	entries, err := sm.LoadSession(sessionKey)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"sessionKey":   sessionKey,
		"size":         info.Size(),
		"lastModified": info.ModTime(),
		"messageCount": len(entries),
	}, nil
}

// Close closes the session manager
func (sm *SessionManager) Close() error {
	// Clear all write locks
	sm.locksMu.Lock()
	sm.writeLocks = make(map[string]*sync.Mutex)
	sm.locksMu.Unlock()

	log.Info().Msg("Session manager closed")

	return nil
}
