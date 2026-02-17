// Package session manages persistent conversation history using JSONL files.
//
// Invariants:
// - Session keys are validated and path-safe.
// - Writes for the same session are serialized.
// - Session append/load/delete operations are observable via tracing and metrics.
//
// Usage:
//
//	mgr, _ := session.New("/tmp/ranya/sessions")
//	_ = mgr.AppendMessage("session:1", session.Message{Role: "user", Content: "hello"})
//	entries, _ := mgr.LoadSession("session:1")
//	_ = entries
package session
