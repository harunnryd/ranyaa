// Package memory indexes workspace markdown content and provides hybrid search.
//
// Invariants:
// - Indexed chunks remain consistent with file content hashes.
// - Search can combine keyword and vector retrieval when embeddings are configured.
// - Sync/search operations emit tracing spans and metrics.
//
// Usage:
//
//	mgr, _ := memory.NewManager(memory.Config{WorkspacePath: "/workspace", DBPath: "/data/memory.db"})
//	defer mgr.Close()
//	_ = mgr.Sync()
//	results, _ := mgr.Search("query", nil)
//	_ = results
package memory
