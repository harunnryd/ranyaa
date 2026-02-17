// Package agent orchestrates session-aware LLM runs with tool loops and provider failover.
//
// Invariants:
// - Agent runs are serialized per session lane through commandqueue.
// - Session state is loaded before execution and persisted after execution.
// - Tool calls route through toolexecutor only.
//
// Usage:
//
//	runner, _ := agent.NewRunner(agent.Config{...})
//	result, _ := runner.Run(agent.AgentRunParams{
//		Prompt: "hello",
//		SessionKey: "session:1",
//		Config: agent.DefaultConfig(),
//	})
//	_ = result
package agent
