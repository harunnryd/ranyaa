package toolexecutor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MCP JSON-RPC messages
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

type mcpError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPServerAdapter implements PluginToolProvider for Model Context Protocol servers
type MCPServerAdapter struct {
	serverID string
	command  string
	args     []string

	mu      sync.Mutex
	process *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner
	id      int
	pending map[int]chan *mcpResponse
}

// NewMCPServerAdapter creates a new adapter for an MCP server
func NewMCPServerAdapter(serverID, command string, args []string) *MCPServerAdapter {
	return &MCPServerAdapter{
		serverID: serverID,
		command:  command,
		args:     args,
		pending:  make(map[int]chan *mcpResponse),
	}
}

// Start starts the MCP server process
func (a *MCPServerAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.process != nil {
		return nil
	}

	cmd := exec.CommandContext(ctx, a.command, a.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	a.process = cmd
	a.stdin = stdin
	a.stdout = bufio.NewScanner(stdout)

	// Listen for responses separately
	go a.listen()

	// Perform initialize handshake (simplified)
	return a.initialize(ctx)
}

func (a *MCPServerAdapter) listen() {
	for a.stdout.Scan() {
		var resp mcpResponse
		if err := json.Unmarshal(a.stdout.Bytes(), &resp); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal MCP response")
			continue
		}

		if id, ok := resp.ID.(float64); ok {
			a.mu.Lock()
			ch, exists := a.pending[int(id)]
			if exists {
				delete(a.pending, int(id))
				ch <- &resp
			}
			a.mu.Unlock()
		}
	}
}

func (a *MCPServerAdapter) initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "Ranya",
			"version": "0.1.0",
		},
	}
	_, err := a.call(ctx, "initialize", params)
	return err
}

func (a *MCPServerAdapter) call(ctx context.Context, method string, params interface{}) (*mcpResponse, error) {
	a.mu.Lock()
	a.id++
	id := a.id
	ch := make(chan *mcpResponse, 1)
	a.pending[id] = ch
	stdin := a.stdin
	a.mu.Unlock()

	req := mcpRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if _, err := io.WriteString(stdin, string(data)+"\n"); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("MCP request timeout")
	}
}

// ExecuteTool executes a tool from the MCP server
func (a *MCPServerAdapter) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (map[string]interface{}, error) {
	if err := a.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	callParams := map[string]interface{}{
		"name":      name,
		"arguments": params,
	}

	resp, err := a.call(ctx, "tools/call", callParams)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// ListResources fetches resource listings from the MCP server.
func (a *MCPServerAdapter) ListResources(ctx context.Context) ([]map[string]interface{}, error) {
	if err := a.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	resp, err := a.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}

	var listResult struct {
		Resources []map[string]interface{} `json:"resources"`
	}
	if err := json.Unmarshal(resp.Result, &listResult); err != nil {
		return nil, err
	}

	return listResult.Resources, nil
}

// ReadResource reads a specific resource from the MCP server.
func (a *MCPServerAdapter) ReadResource(ctx context.Context, uri string) (map[string]interface{}, error) {
	if err := a.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	resp, err := a.call(ctx, "resources/read", map[string]interface{}{"uri": uri})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetTools fetches the tool definitions from the MCP server
func (a *MCPServerAdapter) GetTools(ctx context.Context) ([]ToolDefinition, error) {
	if err := a.Start(ctx); err != nil {
		return nil, err
	}

	resp, err := a.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var listResult struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}

	if err := json.Unmarshal(resp.Result, &listResult); err != nil {
		return nil, err
	}

	defs := make([]ToolDefinition, 0, len(listResult.Tools))
	for _, t := range listResult.Tools {
		def := ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
		}
		if params := parseMCPToolParameters(t.InputSchema); len(params) > 0 {
			def.Parameters = params
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// Stop stops the MCP server process
func (a *MCPServerAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.process != nil && a.process.Process != nil {
		return a.process.Process.Kill()
	}
	return nil
}

func parseMCPToolParameters(schema json.RawMessage) []ToolParameter {
	if len(schema) == 0 {
		return nil
	}

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		return nil
	}

	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	required := make(map[string]bool)
	if reqList, ok := schemaMap["required"].([]interface{}); ok {
		for _, r := range reqList {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}

	params := make([]ToolParameter, 0, len(properties))
	for name, propData := range properties {
		prop, ok := propData.(map[string]interface{})
		if !ok {
			continue
		}
		param := ToolParameter{
			Name:     name,
			Required: required[name],
		}
		if typeVal, ok := prop["type"].(string); ok {
			param.Type = typeVal
		}
		if desc, ok := prop["description"].(string); ok {
			param.Description = desc
		}
		if defVal, ok := prop["default"]; ok {
			param.Default = defVal
		}
		params = append(params, param)
	}

	return params
}
