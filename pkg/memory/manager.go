package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/harun/ranya/internal/observability"
	"github.com/harun/ranya/internal/tracing"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func init() {
	// Auto-register sqlite-vec extension
	sqlite_vec.Auto()
}

// SearchResult represents a search result with relevance score
type SearchResult struct {
	ChunkID      string                 `json:"chunk_id"`
	FilePath     string                 `json:"file_path"`
	Content      string                 `json:"content"`
	Score        float64                `json:"score"`
	VectorScore  *float64               `json:"vector_score,omitempty"`
	KeywordScore *float64               `json:"keyword_score,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// SearchOptions configures search behavior
type SearchOptions struct {
	Limit         int     `json:"limit"`
	VectorWeight  float64 `json:"vector_weight"`
	KeywordWeight float64 `json:"keyword_weight"`
	MinScore      float64 `json:"min_score"`
}

// MemoryStatus represents the current state of the memory manager
type MemoryStatus struct {
	TotalFiles            int        `json:"total_files"`
	TotalChunks           int        `json:"total_chunks"`
	IsDirty               bool       `json:"is_dirty"`
	IsSyncing             bool       `json:"is_syncing"`
	EmbeddingCacheHitRate *float64   `json:"embedding_cache_hit_rate,omitempty"`
	LastSyncTime          *time.Time `json:"last_sync_time,omitempty"`
}

// Manager handles memory indexing and search
type Manager struct {
	db                *sql.DB
	workspacePath     string
	logger            zerolog.Logger
	embeddingProvider EmbeddingProvider
	watcher           *FileWatcher
	mu                sync.RWMutex
	isDirty           bool
	isSyncing         bool
	lastSyncTime      *time.Time
	stats             struct {
		cacheHits   int
		cacheMisses int
	}
}

// Config holds memory manager configuration
type Config struct {
	WorkspacePath     string
	DBPath            string
	Logger            zerolog.Logger
	EmbeddingProvider EmbeddingProvider // Optional, if nil will skip vector search
}

// NewManager creates a new memory manager
func NewManager(cfg Config) (*Manager, error) {
	observability.EnsureRegistered()

	if cfg.WorkspacePath == "" {
		return nil, errors.New("workspace path is required")
	}
	if cfg.DBPath == "" {
		return nil, errors.New("database path is required")
	}

	// Open database with FTS5 support
	db, err := sql.Open("sqlite3", cfg.DBPath+"?_fts5=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	m := &Manager{
		db:                db,
		workspacePath:     cfg.WorkspacePath,
		logger:            cfg.Logger,
		embeddingProvider: cfg.EmbeddingProvider,
		isDirty:           true, // Start dirty to trigger initial sync
	}

	// Initialize schema
	if err := m.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Initialize file watcher
	watcher, err := NewFileWatcher(cfg.Logger, func() {
		m.MarkDirty()
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Watch workspace directory
	if err := watcher.Watch(cfg.WorkspacePath); err != nil {
		watcher.Stop()
		db.Close()
		return nil, fmt.Errorf("failed to watch workspace: %w", err)
	}

	m.watcher = watcher

	m.logger.Info().Msg("Memory manager initialized")
	return m, nil
}

// initSchema creates database tables
func (m *Manager) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			content_hash TEXT NOT NULL,
			indexed_at INTEGER NOT NULL,
			size_bytes INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
		CREATE INDEX IF NOT EXISTS idx_files_hash ON files(content_hash);

		CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			file_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			start_offset INTEGER NOT NULL,
			end_offset INTEGER NOT NULL,
			start_line INTEGER,
			end_line INTEGER,
			heading TEXT,
			FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_id);

		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			chunk_id UNINDEXED,
			content,
			tokenize='porter unicode61'
		);

		CREATE TABLE IF NOT EXISTS embedding_cache (
			content_hash TEXT PRIMARY KEY,
			embedding BLOB NOT NULL,
			dimension INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_cache_created ON embedding_cache(created_at);

		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`

	if _, err := m.db.Exec(schema); err != nil {
		return err
	}

	// Create vector table if embedding provider is available
	if m.embeddingProvider != nil {
		dimension := m.embeddingProvider.Dimension()
		vectorSchema := fmt.Sprintf(`
			CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vec0(
				chunk_id TEXT PRIMARY KEY,
				embedding float[%d] distance_metric=cosine
			);
		`, dimension)

		if _, err := m.db.Exec(vectorSchema); err != nil {
			return fmt.Errorf("failed to create vector table: %w", err)
		}
	}

	return nil
}

// Search performs hybrid search (vector + keyword)
func (m *Manager) Search(query string, opts *SearchOptions) ([]SearchResult, error) {
	return m.SearchWithContext(context.Background(), query, opts)
}

// SearchWithContext performs hybrid search (vector + keyword) with context propagation.
func (m *Manager) SearchWithContext(ctx context.Context, query string, opts *SearchOptions) ([]SearchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, span := tracing.StartSpan(
		ctx,
		"ranya.memory",
		"memory.search",
		attribute.String("query", query),
	)
	defer span.End()

	logger := tracing.LoggerFromContext(ctx, m.logger)
	start := time.Now()
	defer observability.RecordMemorySearch(time.Since(start))

	if query == "" {
		return []SearchResult{}, nil
	}

	// Set defaults
	if opts == nil {
		opts = &SearchOptions{
			Limit:         20,
			VectorWeight:  0.7,
			KeywordWeight: 0.3,
		}
	}

	// Sync if dirty
	m.mu.RLock()
	dirty := m.isDirty
	m.mu.RUnlock()

	if dirty {
		if err := m.Sync(); err != nil {
			m.logger.Warn().Err(err).Msg("Sync failed before search")
		}
	}

	// Perform vector and keyword search in parallel
	var vectorResults []vectorSearchResult
	var keywordResults []keywordSearchResult
	var vectorErr, keywordErr error

	var wg sync.WaitGroup
	wg.Add(2)

	// Vector search
	go func() {
		defer wg.Done()
		if m.embeddingProvider != nil {
			vectorResults, vectorErr = m.vectorSearch(ctx, query, 200)
		}
	}()

	// Keyword search
	go func() {
		defer wg.Done()
		keywordResults, keywordErr = m.keywordSearch(query, 200)
	}()

	wg.Wait()

	// Handle errors with graceful degradation
	if vectorErr != nil {
		logger.Warn().Err(vectorErr).Msg("Vector search failed, using keyword only")
	}
	if keywordErr != nil {
		logger.Warn().Err(keywordErr).Msg("Keyword search failed, using vector only")
	}

	if vectorErr != nil && keywordErr != nil {
		span.RecordError(vectorErr)
		span.RecordError(keywordErr)
		span.SetStatus(codes.Error, "both search methods failed")
		return nil, fmt.Errorf("both search methods failed")
	}

	// Merge results
	results := m.mergeResults(vectorResults, keywordResults, opts)

	// Limit results
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	logger.Debug().
		Str("query", query).
		Int("results", len(results)).
		Msg("Search completed")

	return results, nil
}

type vectorSearchResult struct {
	chunkID    string
	similarity float64 // cosine similarity (-1 to 1)
}

type keywordSearchResult struct {
	chunkID   string
	bm25Score float64
}

// vectorSearch performs vector similarity search
func (m *Manager) vectorSearch(ctx context.Context, query string, limit int) ([]vectorSearchResult, error) {
	// Generate query embedding
	embedding, err := m.embeddingProvider.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Convert embedding to JSON array string
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	// Query vector table using vec_distance_cosine
	sql := `
		SELECT 
			chunk_id,
			vec_distance_cosine(embedding, ?) as distance
		FROM embeddings
		ORDER BY distance ASC
		LIMIT ?
	`

	rows, err := m.db.QueryContext(ctx, sql, string(embeddingJSON), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []vectorSearchResult
	for rows.Next() {
		var chunkID string
		var distance float64
		if err := rows.Scan(&chunkID, &distance); err != nil {
			return nil, err
		}

		// Convert distance to similarity: similarity = 1 - (distance / 2)
		// cosine distance range is [0, 2], we want similarity in [-1, 1]
		similarity := 1.0 - distance

		results = append(results, vectorSearchResult{
			chunkID:    chunkID,
			similarity: similarity,
		})
	}

	return results, nil
}

// keywordSearch performs FTS5 keyword search
func (m *Manager) keywordSearch(query string, limit int) ([]keywordSearchResult, error) {
	// Use FTS5 MATCH query
	sql := `
		SELECT chunk_id, bm25(chunks_fts) as score
		FROM chunks_fts
		WHERE chunks_fts MATCH ?
		ORDER BY score
		LIMIT ?
	`

	rows, err := m.db.Query(sql, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []keywordSearchResult
	for rows.Next() {
		var chunkID string
		var score float64
		if err := rows.Scan(&chunkID, &score); err != nil {
			return nil, err
		}

		// BM25 scores are negative, convert to positive
		results = append(results, keywordSearchResult{
			chunkID:   chunkID,
			bm25Score: -score,
		})
	}

	return results, nil
}

// mergeResults combines vector and keyword search results
func (m *Manager) mergeResults(vectorResults []vectorSearchResult, keywordResults []keywordSearchResult, opts *SearchOptions) []SearchResult {
	// Create maps for quick lookup
	vectorMap := make(map[string]float64)
	keywordMap := make(map[string]float64)

	// Find max scores for normalization
	var maxVector, maxKeyword float64
	for _, r := range vectorResults {
		vectorMap[r.chunkID] = r.similarity
		if r.similarity > maxVector {
			maxVector = r.similarity
		}
	}
	for _, r := range keywordResults {
		keywordMap[r.chunkID] = r.bm25Score
		if r.bm25Score > maxKeyword {
			maxKeyword = r.bm25Score
		}
	}

	// Collect all unique chunk IDs
	chunkIDs := make(map[string]bool)
	for id := range vectorMap {
		chunkIDs[id] = true
	}
	for id := range keywordMap {
		chunkIDs[id] = true
	}

	// Calculate combined scores
	type scoredResult struct {
		chunkID      string
		score        float64
		vectorScore  *float64
		keywordScore *float64
	}

	var scored []scoredResult
	for chunkID := range chunkIDs {
		var normalizedVector, normalizedKeyword float64

		// Normalize vector score: map similarity [-1, 1] to [0, 1]
		if vectorScore, ok := vectorMap[chunkID]; ok {
			normalizedVector = (vectorScore + 1) / 2
		}

		// Normalize keyword score
		if keywordScore, ok := keywordMap[chunkID]; ok && maxKeyword > 0 {
			normalizedKeyword = keywordScore / maxKeyword
		}

		// Combine scores with weights
		combinedScore := (normalizedVector * opts.VectorWeight) + (normalizedKeyword * opts.KeywordWeight)

		// Apply minimum score threshold
		if opts.MinScore > 0 && combinedScore < opts.MinScore {
			continue
		}

		var vecPtr, keyPtr *float64
		if _, ok := vectorMap[chunkID]; ok {
			v := normalizedVector
			vecPtr = &v
		}
		if _, ok := keywordMap[chunkID]; ok {
			k := normalizedKeyword
			keyPtr = &k
		}

		scored = append(scored, scoredResult{
			chunkID:      chunkID,
			score:        combinedScore,
			vectorScore:  vecPtr,
			keywordScore: keyPtr,
		})
	}

	// Sort by combined score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Fetch chunk details
	results := make([]SearchResult, 0, len(scored))
	for _, s := range scored {
		var content, filePath string
		err := m.db.QueryRow(`
			SELECT c.content, f.path
			FROM chunks c
			JOIN files f ON c.file_id = f.id
			WHERE c.id = ?
		`, s.chunkID).Scan(&content, &filePath)

		if err != nil {
			m.logger.Warn().Err(err).Str("chunkID", s.chunkID).Msg("Failed to fetch chunk details")
			continue
		}

		results = append(results, SearchResult{
			ChunkID:      s.chunkID,
			FilePath:     filePath,
			Content:      content,
			Score:        s.score,
			VectorScore:  s.vectorScore,
			KeywordScore: s.keywordScore,
		})
	}

	return results
}

// Sync indexes the workspace
func (m *Manager) Sync() error {
	ctx := context.Background()
	ctx, span := tracing.StartSpan(ctx, "ranya.memory", "memory.sync")
	defer span.End()
	logger := tracing.LoggerFromContext(ctx, m.logger)

	m.mu.Lock()
	if m.isSyncing {
		m.mu.Unlock()
		span.SetStatus(codes.Error, "sync already in progress")
		return errors.New("sync already in progress")
	}
	m.isSyncing = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.isSyncing = false
		m.isDirty = false
		now := time.Now()
		m.lastSyncTime = &now
		m.mu.Unlock()
	}()

	logger.Info().Msg("Starting sync")
	start := time.Now()
	defer observability.RecordMemoryWrite(time.Since(start))

	// Find all markdown files
	var mdFiles []string
	err := filepath.WalkDir(m.workspacePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			span.RecordError(err)
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			relPath, _ := filepath.Rel(m.workspacePath, path)
			mdFiles = append(mdFiles, relPath)
		}
		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("failed to walk workspace: %w", err)
	}

	// Index each file
	filesIndexed := 0
	filesSkipped := 0
	chunksCreated := 0

	for _, relPath := range mdFiles {
		fullPath := filepath.Join(m.workspacePath, relPath)
		indexed, chunks, err := m.indexFile(fullPath, relPath)
		if err != nil {
			logger.Warn().Err(err).Str("file", relPath).Msg("Failed to index file")
			span.RecordError(err)
			continue
		}
		if indexed {
			filesIndexed++
			chunksCreated += chunks
		} else {
			filesSkipped++
		}
	}

	// Prune deleted files
	pruned, err := m.pruneDeletedFiles(mdFiles)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to prune deleted files")
		span.RecordError(err)
	}

	duration := time.Since(start)
	logger.Info().
		Int("files_indexed", filesIndexed).
		Int("files_skipped", filesSkipped).
		Int("chunks_created", chunksCreated).
		Int("files_pruned", pruned).
		Dur("duration", duration).
		Msg("Sync completed")

	status := m.Status()
	observability.SetMemoryEntries(status.TotalChunks)

	return nil
}

// indexFile indexes a single file
func (m *Manager) indexFile(fullPath, relPath string) (bool, int, error) {
	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return false, 0, err
	}

	// Compute content hash
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])

	// Check if file needs reindexing
	var existingHash string
	err = m.db.QueryRow("SELECT content_hash FROM files WHERE path = ?", relPath).Scan(&existingHash)
	if err == nil && existingHash == contentHash {
		// File unchanged, skip
		return false, 0, nil
	}

	// Start transaction
	tx, err := m.db.Begin()
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback()

	// Delete existing file entry (cascades to chunks)
	if _, err := tx.Exec("DELETE FROM files WHERE path = ?", relPath); err != nil {
		return false, 0, err
	}

	// Insert file record
	stat, _ := os.Stat(fullPath)
	result, err := tx.Exec(
		"INSERT INTO files (path, content_hash, indexed_at, size_bytes) VALUES (?, ?, ?, ?)",
		relPath, contentHash, time.Now().Unix(), stat.Size(),
	)
	if err != nil {
		return false, 0, err
	}

	fileID, _ := result.LastInsertId()

	// Chunk content
	chunks := m.chunkContent(string(content))

	// Insert chunks and generate embeddings
	ctx := context.Background()
	for i, chunk := range chunks {
		chunkID := fmt.Sprintf("%s#%d", relPath, i)

		// Insert into chunks table
		_, err := tx.Exec(
			"INSERT INTO chunks (id, file_id, content, start_offset, end_offset) VALUES (?, ?, ?, ?, ?)",
			chunkID, fileID, chunk.content, chunk.startOffset, chunk.endOffset,
		)
		if err != nil {
			return false, 0, err
		}

		// Insert into FTS5 table
		_, err = tx.Exec(
			"INSERT INTO chunks_fts (chunk_id, content) VALUES (?, ?)",
			chunkID, chunk.content,
		)
		if err != nil {
			return false, 0, err
		}

		// Generate and store embedding if provider available
		if m.embeddingProvider != nil {
			if err := m.storeEmbedding(ctx, tx, chunkID, chunk.content); err != nil {
				m.logger.Warn().Err(err).Str("chunk", chunkID).Msg("Failed to store embedding")
				// Continue processing other chunks
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return false, 0, err
	}

	return true, len(chunks), nil
}

// storeEmbedding generates and stores embedding for a chunk
func (m *Manager) storeEmbedding(ctx context.Context, tx *sql.Tx, chunkID, content string) error {
	// Check cache first
	contentHashBytes := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(contentHashBytes[:])

	var cachedEmbedding []byte
	err := tx.QueryRow("SELECT embedding FROM embedding_cache WHERE content_hash = ?", contentHash).Scan(&cachedEmbedding)

	var embedding []float32
	if err == nil {
		// Cache hit
		m.stats.cacheHits++
		if err := json.Unmarshal(cachedEmbedding, &embedding); err != nil {
			return fmt.Errorf("failed to unmarshal cached embedding: %w", err)
		}
	} else {
		// Cache miss - generate embedding
		m.stats.cacheMisses++
		embedding, err = m.embeddingProvider.GenerateEmbedding(ctx, content)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}

		// Store in cache
		embeddingJSON, err := json.Marshal(embedding)
		if err != nil {
			return fmt.Errorf("failed to marshal embedding: %w", err)
		}

		_, err = tx.Exec(
			"INSERT OR REPLACE INTO embedding_cache (content_hash, embedding, dimension, created_at) VALUES (?, ?, ?, ?)",
			contentHash, embeddingJSON, len(embedding), time.Now().Unix(),
		)
		if err != nil {
			return fmt.Errorf("failed to cache embedding: %w", err)
		}
	}

	// Store in vector table
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding for storage: %w", err)
	}

	_, err = tx.Exec(
		"INSERT OR REPLACE INTO embeddings (chunk_id, embedding) VALUES (?, ?)",
		chunkID, string(embeddingJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to store embedding in vector table: %w", err)
	}

	return nil
}

type chunk struct {
	content     string
	startOffset int
	endOffset   int
}

// chunkContent splits content into chunks
func (m *Manager) chunkContent(content string) []chunk {
	const minSize = 500
	const maxSize = 1000
	const overlap = 50

	var chunks []chunk
	lines := strings.Split(content, "\n")

	var currentChunk strings.Builder
	startOffset := 0
	currentOffset := 0

	for _, line := range lines {
		lineLen := len(line) + 1 // +1 for newline

		// If adding this line exceeds maxSize and we have content, create chunk
		if currentChunk.Len() > 0 && currentChunk.Len()+lineLen > maxSize {
			chunks = append(chunks, chunk{
				content:     strings.TrimSpace(currentChunk.String()),
				startOffset: startOffset,
				endOffset:   currentOffset,
			})

			// Start new chunk with overlap
			chunkText := currentChunk.String()
			if len(chunkText) > overlap {
				overlapText := chunkText[len(chunkText)-overlap:]
				currentChunk.Reset()
				currentChunk.WriteString(overlapText)
				startOffset = currentOffset - overlap
			} else {
				currentChunk.Reset()
				startOffset = currentOffset
			}
		}

		currentChunk.WriteString(line)
		currentChunk.WriteString("\n")
		currentOffset += lineLen
	}

	// Add final chunk if it meets minSize or is the only chunk
	if currentChunk.Len() >= minSize || len(chunks) == 0 {
		if currentChunk.Len() > 0 {
			chunks = append(chunks, chunk{
				content:     strings.TrimSpace(currentChunk.String()),
				startOffset: startOffset,
				endOffset:   currentOffset,
			})
		}
	}

	return chunks
}

// pruneDeletedFiles removes files that no longer exist
func (m *Manager) pruneDeletedFiles(existingFiles []string) (int, error) {
	// Get all indexed files
	rows, err := m.db.Query("SELECT path FROM files")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	existingSet := make(map[string]bool)
	for _, f := range existingFiles {
		existingSet[f] = true
	}

	var toDelete []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		if !existingSet[path] {
			toDelete = append(toDelete, path)
		}
	}

	// Delete files
	for _, path := range toDelete {
		if _, err := m.db.Exec("DELETE FROM files WHERE path = ?", path); err != nil {
			return 0, err
		}
	}

	return len(toDelete), nil
}

// Status returns current memory manager status
func (m *Manager) Status() MemoryStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var status MemoryStatus
	status.IsDirty = m.isDirty
	status.IsSyncing = m.isSyncing
	status.LastSyncTime = m.lastSyncTime

	// Count files
	m.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&status.TotalFiles)

	// Count chunks
	m.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&status.TotalChunks)

	// Calculate cache hit rate
	total := m.stats.cacheHits + m.stats.cacheMisses
	if total > 0 {
		rate := float64(m.stats.cacheHits) / float64(total)
		status.EmbeddingCacheHitRate = &rate
	}

	return status
}

// Close closes the memory manager
func (m *Manager) Close() error {
	m.logger.Info().Msg("Closing memory manager")

	if m.watcher != nil {
		m.watcher.Stop()
	}

	return m.db.Close()
}

// MarkDirty marks the index as needing sync
func (m *Manager) MarkDirty() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isDirty = true
}
