// Package toolexecutor registers and executes structured tools for agents.
//
// Invariants:
// - Tool names are unique.
// - Parameters are schema-validated before execution.
// - Plugin tools can require approval based on declared permissions.
//
// Usage:
//
//	exec := toolexecutor.New()
//	_ = exec.RegisterTool(toolexecutor.ToolDefinition{
//		Name: "echo",
//		Description: "Echo input",
//		Parameters: []toolexecutor.ToolParameter{{Name: "text", Type: "string", Description: "text", Required: true}},
//		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) { return params["text"], nil },
//	})
package toolexecutor
