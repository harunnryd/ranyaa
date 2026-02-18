package workspace

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/harun/ranya/internal/config"
)

// ParseAgentsMarkdown parses an AGENTS.md content into a slice of AgentConfig
func ParseAgentsMarkdown(content string) []config.AgentConfig {
	var agents []config.AgentConfig

	// Split by H2 headers (## agent-id)
	re := regexp.MustCompile(`(?m)^##\s+([^\n\r]+)`)
	matches := re.FindAllStringSubmatchIndex(content, -1)

	for i, match := range matches {
		id := strings.TrimSpace(content[match[2]:match[3]])
		var agentBlock string
		if i+1 < len(matches) {
			agentBlock = content[match[1]:matches[i+1][0]]
		} else {
			agentBlock = content[match[1]:]
		}

		agents = append(agents, parseAgentBlock(id, agentBlock))
	}

	return agents
}

func parseAgentBlock(id, content string) config.AgentConfig {
	agent := config.AgentConfig{
		ID: id,
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			// Check for multiline SystemPrompt if it started
			if agent.SystemPrompt != "" && strings.HasPrefix(line, "  ") {
				agent.SystemPrompt += "\n" + strings.TrimSpace(line)
			}
			continue
		}

		parts := strings.SplitN(line[2:], ":", 2)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			agent.Name = val
		case "role":
			agent.Role = val
		case "model":
			agent.Model = val
		case "temperature":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				agent.Temperature = f
			}
		case "maxtokens":
			if i, err := strconv.Atoi(val); err == nil {
				agent.MaxTokens = i
			}
		case "tools":
			tools := strings.Split(val, ",")
			for _, t := range tools {
				agent.Tools.Allow = append(agent.Tools.Allow, strings.TrimSpace(t))
			}
		case "sandbox":
			agent.Sandbox.Runtime = val
		case "systemprompt":
			agent.SystemPrompt = val
		}
	}

	return agent
}

// ParseSoulMarkdown parses a SOUL.md content into a personality string or map
func ParseSoulMarkdown(content string) string {
	// For now, just return the whole trimmed content as a system prompt addition
	return strings.TrimSpace(content)
}
