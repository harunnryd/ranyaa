package coretools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harun/ranya/pkg/sandbox"
	"github.com/harun/ranya/pkg/toolexecutor"
)

// Options configures core tool registration.
type Options struct {
	WorkspaceRoot string
	SandboxManager *toolexecutor.SandboxManager
}

type hunkLine struct {
	kind byte
	text string
}

type hunk struct {
	start int
	lines []hunkLine
}

type filePatch struct {
	path  string
	hunks []hunk
}

// RegisterCoreTools registers baseline runtime and filesystem tools.
func RegisterCoreTools(executor *toolexecutor.ToolExecutor, opts Options) error {
	if executor == nil {
		return errors.New("tool executor is required")
	}

	tools := []toolexecutor.ToolDefinition{
		execTool(opts),
		readFileTool(opts),
		writeFileTool(opts),
		editFileTool(opts),
		applyPatchTool(opts),
	}

	for _, tool := range tools {
		if err := executor.RegisterTool(tool); err != nil {
			return fmt.Errorf("failed to register tool %s: %w", tool.Name, err)
		}
	}
	return nil
}

func execTool(opts Options) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "exec",
		Description: "Execute a shell command in a sandboxed runtime when configured.",
		Category:    toolexecutor.CategoryShell,
		SandboxRequired: true,
		ApprovalRequired: true,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "command", Type: "string", Description: "Command to execute", Required: true},
			{Name: "args", Type: "array", Description: "Command arguments", Required: false},
			{Name: "cwd", Type: "string", Description: "Working directory (relative to workspace)", Required: false},
			{Name: "timeout", Type: "number", Description: "Timeout in seconds", Required: false},
			{Name: "env", Type: "object", Description: "Environment variables", Required: false},
			{Name: "stdin", Type: "string", Description: "Standard input", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			execCtx := toolexecutor.ExecContextFromContext(ctx)
			if execCtx == nil {
				return nil, fmt.Errorf("execution context is required")
			}
			if opts.SandboxManager == nil {
				return nil, fmt.Errorf("sandbox manager is required")
			}

			command, _ := params["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				return nil, fmt.Errorf("command is required")
			}

			args := toStringSlice(params["args"])
			timeout := parseDurationSeconds(params["timeout"], 30*time.Second)
			env := toStringMap(params["env"])

			workspaceRoot, err := resolveWorkspaceRoot(execCtx, opts)
			if err != nil {
				return nil, err
			}
			cwd := resolveWorkspacePath(workspaceRoot, params["cwd"])

			req := sandbox.ExecuteRequest{
				Command:    command,
				Args:       args,
				Env:        env,
				WorkingDir: cwd,
				Timeout:    timeout,
			}
			if stdin, ok := params["stdin"].(string); ok && stdin != "" {
				req.Stdin = []byte(stdin)
			}

			key := sandboxKeyFromExecContext(execCtx)
			res, err := opts.SandboxManager.ExecuteInSandbox(ctx, key, req)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"stdout":    string(res.Stdout),
				"stderr":    string(res.Stderr),
				"exit_code": res.ExitCode,
				"duration":  res.Duration.Milliseconds(),
			}, nil
		},
	}
}

func readFileTool(opts Options) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "read_file",
		Description: "Read a file from the workspace.",
		Category:    toolexecutor.CategoryRead,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "path", Type: "string", Description: "Relative file path", Required: true},
			{Name: "max_bytes", Type: "number", Description: "Maximum bytes to read (default 200000)", Required: false, Default: 200000},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			execCtx := toolexecutor.ExecContextFromContext(ctx)
			workspaceRoot, err := resolveWorkspaceRoot(execCtx, opts)
			if err != nil {
				return nil, err
			}
			pathValue, _ := params["path"].(string)
			target, err := resolvePathInWorkspace(workspaceRoot, pathValue)
			if err != nil {
				return nil, err
			}

			maxBytes := int64(200000)
			if raw, ok := params["max_bytes"].(float64); ok && raw > 0 {
				maxBytes = int64(raw)
			}

			data, truncated, err := readFileWithLimit(target, maxBytes)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"path":      pathValue,
				"content":   string(data),
				"truncated": truncated,
				"bytes":     len(data),
			}, nil
		},
	}
}

func writeFileTool(opts Options) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file in the workspace.",
		Category:    toolexecutor.CategoryWrite,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "path", Type: "string", Description: "Relative file path", Required: true},
			{Name: "content", Type: "string", Description: "File content", Required: true},
			{Name: "append", Type: "boolean", Description: "Append to file (default false)", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			execCtx := toolexecutor.ExecContextFromContext(ctx)
			workspaceRoot, err := resolveWorkspaceRoot(execCtx, opts)
			if err != nil {
				return nil, err
			}
			pathValue, _ := params["path"].(string)
			target, err := resolvePathInWorkspace(workspaceRoot, pathValue)
			if err != nil {
				return nil, err
			}
			content, _ := params["content"].(string)
			appendMode, _ := params["append"].(bool)

			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return nil, err
			}

			if useSandbox(execCtx) && opts.SandboxManager != nil {
				if err := writeFileInSandbox(ctx, opts.SandboxManager, execCtx, target, content, appendMode); err != nil {
					return nil, err
				}
			} else {
				flag := os.O_CREATE | os.O_WRONLY
				if appendMode {
					flag |= os.O_APPEND
				} else {
					flag |= os.O_TRUNC
				}
				if err := os.WriteFile(target, []byte(content), 0644); err != nil {
					return nil, err
				}
			}

			return map[string]interface{}{
				"path":   pathValue,
				"bytes":  len(content),
				"append": appendMode,
			}, nil
		},
	}
}

func editFileTool(opts Options) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "edit_file",
		Description: "Replace text in a workspace file.",
		Category:    toolexecutor.CategoryWrite,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "path", Type: "string", Description: "Relative file path", Required: true},
			{Name: "search", Type: "string", Description: "Text to search for", Required: true},
			{Name: "replace", Type: "string", Description: "Replacement text", Required: true},
			{Name: "replace_all", Type: "boolean", Description: "Replace all occurrences (default false)", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			execCtx := toolexecutor.ExecContextFromContext(ctx)
			workspaceRoot, err := resolveWorkspaceRoot(execCtx, opts)
			if err != nil {
				return nil, err
			}
			pathValue, _ := params["path"].(string)
			target, err := resolvePathInWorkspace(workspaceRoot, pathValue)
			if err != nil {
				return nil, err
			}
			search, _ := params["search"].(string)
			replace, _ := params["replace"].(string)
			replaceAll, _ := params["replace_all"].(bool)
			if search == "" {
				return nil, fmt.Errorf("search is required")
			}

			data, err := os.ReadFile(target)
			if err != nil {
				return nil, err
			}
			content := string(data)

			var updated string
			occurrences := 0
			if replaceAll {
				occurrences = strings.Count(content, search)
				updated = strings.ReplaceAll(content, search, replace)
			} else {
				if idx := strings.Index(content, search); idx >= 0 {
					occurrences = 1
					updated = content[:idx] + replace + content[idx+len(search):]
				} else {
					updated = content
				}
			}
			if occurrences == 0 {
				return nil, fmt.Errorf("search text not found")
			}

			if useSandbox(execCtx) && opts.SandboxManager != nil {
				if err := writeFileInSandbox(ctx, opts.SandboxManager, execCtx, target, updated, false); err != nil {
					return nil, err
				}
			} else if err := os.WriteFile(target, []byte(updated), 0644); err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"path":        pathValue,
				"occurrences": occurrences,
			}, nil
		},
	}
}

func applyPatchTool(opts Options) toolexecutor.ToolDefinition {
	return toolexecutor.ToolDefinition{
		Name:        "apply_patch",
		Description: "Apply a unified diff patch within the workspace.",
		Category:    toolexecutor.CategoryWrite,
		Parameters: []toolexecutor.ToolParameter{
			{Name: "patch", Type: "string", Description: "Unified diff patch", Required: true},
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			execCtx := toolexecutor.ExecContextFromContext(ctx)
			workspaceRoot, err := resolveWorkspaceRoot(execCtx, opts)
			if err != nil {
				return nil, err
			}
			patchText, _ := params["patch"].(string)
			if strings.TrimSpace(patchText) == "" {
				return nil, fmt.Errorf("patch is required")
			}

			results, err := applyUnifiedPatch(workspaceRoot, patchText)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"files": results,
			}, nil
		},
	}
}

type patchApplyResult struct {
	Path        string `json:"path"`
	Applied     bool   `json:"applied"`
	HunksApplied int   `json:"hunks_applied"`
}

func applyUnifiedPatch(workspaceRoot string, patchText string) ([]patchApplyResult, error) {
	var patches []filePatch
	lines := strings.Split(patchText, "\n")
	var current *filePatch
	var currentHunk *hunk

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "--- ") {
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			path = strings.TrimPrefix(path, "a/")
			path = strings.TrimPrefix(path, "b/")
			if path == "" {
				continue
			}
			patches = append(patches, filePatch{path: path})
			current = &patches[len(patches)-1]
			currentHunk = nil
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if current == nil {
				continue
			}
			start, err := parseUnifiedHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current.hunks = append(current.hunks, hunk{start: start})
			currentHunk = &current.hunks[len(current.hunks)-1]
			continue
		}
		if currentHunk == nil || len(line) == 0 {
			continue
		}
		switch line[0] {
		case ' ', '+', '-':
			currentHunk.lines = append(currentHunk.lines, hunkLine{kind: line[0], text: line[1:]})
		default:
		}
	}

	results := make([]patchApplyResult, 0, len(patches))
	for _, patch := range patches {
		target, err := resolvePathInWorkspace(workspaceRoot, patch.path)
		if err != nil {
			return nil, err
		}
		orig, err := os.ReadFile(target)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		origLines := splitLines(string(orig))
		newLines, hunksApplied, err := applyHunks(origLines, patch.hunks)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
			return nil, err
		}
		results = append(results, patchApplyResult{
			Path:         patch.path,
			Applied:      true,
			HunksApplied: hunksApplied,
		})
	}

	return results, nil
}

func parseUnifiedHunkHeader(line string) (int, error) {
	// format: @@ -start,count +start,count @@
	parts := strings.Split(line, " ")
	if len(parts) < 3 {
		return 0, fmt.Errorf("invalid hunk header: %s", line)
	}
	left := strings.TrimPrefix(parts[1], "-")
	fields := strings.Split(left, ",")
	start := fields[0]
	var startInt int
	if _, err := fmt.Sscanf(start, "%d", &startInt); err != nil {
		return 0, err
	}
	if startInt < 1 {
		startInt = 1
	}
	return startInt, nil
}

func applyHunks(orig []string, hunks []hunk) ([]string, int, error) {
	out := make([]string, 0, len(orig))
	idx := 0
	applied := 0

	for _, h := range hunks {
		target := h.start - 1
		if target < 0 {
			target = 0
		}
		if target > len(orig) {
			target = len(orig)
		}
		out = append(out, orig[idx:target]...)
		idx = target

		for _, ln := range h.lines {
			switch ln.kind {
			case ' ':
				if idx >= len(orig) || orig[idx] != ln.text {
					return nil, applied, fmt.Errorf("context mismatch at line %d", idx+1)
				}
				out = append(out, orig[idx])
				idx++
			case '-':
				if idx >= len(orig) || orig[idx] != ln.text {
					return nil, applied, fmt.Errorf("delete mismatch at line %d", idx+1)
				}
				idx++
			case '+':
				out = append(out, ln.text)
			}
		}
		applied++
	}

	out = append(out, orig[idx:]...)
	return out, applied, nil
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	return lines
}

func readFileWithLimit(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	var buf bytes.Buffer
	truncated := false
	if limit <= 0 {
		limit = 200000
	}
	if _, err := io.CopyN(&buf, file, limit); err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	if extra := make([]byte, 1); true {
		if _, err := file.Read(extra); err == nil {
			truncated = true
		}
	}
	return buf.Bytes(), truncated, nil
}

func writeFileInSandbox(ctx context.Context, sm *toolexecutor.SandboxManager, execCtx *toolexecutor.ExecutionContext, target string, content string, appendMode bool) error {
	if sm == nil {
		return fmt.Errorf("sandbox manager is required")
	}
	args := []string{target}
	command := "tee"
	if appendMode {
		args = []string{"-a", target}
	}
	req := sandbox.ExecuteRequest{
		Command:    command,
		Args:       args,
		WorkingDir: filepath.Dir(target),
		Stdin:      []byte(content),
		Timeout:    30 * time.Second,
	}
	key := sandboxKeyFromExecContext(execCtx)
	res, err := sm.ExecuteInSandbox(ctx, key, req)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("sandbox write failed: %s", string(res.Stderr))
	}
	return nil
}

func resolveWorkspaceRoot(execCtx *toolexecutor.ExecutionContext, opts Options) (string, error) {
	if execCtx != nil && strings.TrimSpace(execCtx.WorkingDir) != "" {
		return filepath.Clean(execCtx.WorkingDir), nil
	}
	if strings.TrimSpace(opts.WorkspaceRoot) != "" {
		return filepath.Clean(opts.WorkspaceRoot), nil
	}
	return "", fmt.Errorf("workspace root is not configured")
}

func resolveWorkspacePath(workspaceRoot string, value interface{}) string {
	raw, _ := value.(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return workspaceRoot
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(workspaceRoot, raw))
}

func resolvePathInWorkspace(workspaceRoot string, pathValue string) (string, error) {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(pathValue, "://") {
		return "", fmt.Errorf("path must be a local file")
	}
	candidate := pathValue
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspaceRoot, candidate)
	}
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(workspaceRoot, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return candidate, nil
	}
	return "", fmt.Errorf("path %q is outside workspace root", pathValue)
}

func useSandbox(execCtx *toolexecutor.ExecutionContext) bool {
	if execCtx == nil || execCtx.SandboxPolicy == nil {
		return false
	}
	if mode, ok := execCtx.SandboxPolicy["mode"].(string); ok {
		return mode == "all" || mode == "tools"
	}
	return false
}

func sandboxKeyFromExecContext(execCtx *toolexecutor.ExecutionContext) string {
	if execCtx == nil {
		return "global"
	}
	scope := "session"
	if execCtx.SandboxPolicy != nil {
		if rawScope, ok := execCtx.SandboxPolicy["scope"].(string); ok && rawScope != "" {
			scope = rawScope
		}
	}
	if scope == "agent" && execCtx.AgentID != "" {
		return execCtx.AgentID
	}
	if execCtx.SessionKey != "" {
		return execCtx.SessionKey
	}
	if execCtx.AgentID != "" {
		return execCtx.AgentID
	}
	return "global"
}

func toStringSlice(value interface{}) []string {
	raw, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toStringMap(value interface{}) map[string]string {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch typed := v.(type) {
		case string:
			out[k] = typed
		default:
			b, _ := json.Marshal(typed)
			out[k] = string(b)
		}
	}
	return out
}

func parseDurationSeconds(value interface{}, fallback time.Duration) time.Duration {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return time.Duration(v * float64(time.Second))
		}
	case int:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case int64:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	}
	return fallback
}
