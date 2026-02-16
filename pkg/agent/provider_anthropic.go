package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements LLMProvider for Anthropic Claude
type AnthropicProvider struct {
	client anthropic.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// Provider returns the provider name
func (p *AnthropicProvider) Provider() string {
	return "anthropic"
}

// Call makes an API call to Anthropic Claude
func (p *AnthropicProvider) Call(ctx context.Context, request LLMRequest) (*LLMResponse, error) {
	// Convert messages to Anthropic format
	anthropicMessages := []anthropic.MessageParam{}

	for _, msg := range request.Messages {
		if msg.Role == "system" {
			continue // System messages handled separately
		}

		// Handle tool results
		if msg.Role == "tool" {
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
			continue
		}

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			blocks := []anthropic.ContentBlockParamUnion{}
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Parameters, tc.Name))
			}
			anthropicMessages = append(anthropicMessages, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: blocks,
			})
			continue
		}

		// Handle regular messages
		if msg.Role == "user" {
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		} else if msg.Role == "assistant" {
			anthropicMessages = append(anthropicMessages, anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock(msg.Content),
				},
			})
		}
	}

	// Build request parameters
	reqParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(request.Model),
		Messages:  anthropicMessages,
		MaxTokens: int64(request.MaxTokens),
	}

	// Add system prompt if provided
	if request.SystemPrompt != "" {
		reqParams.System = []anthropic.TextBlockParam{
			{Text: request.SystemPrompt},
		}
	}

	// Add temperature if provided
	if request.Temperature > 0 {
		reqParams.Temperature = anthropic.Float(request.Temperature)
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools := []anthropic.ToolUnionParam{}
		for _, tool := range request.Tools {
			toolMap := tool.(map[string]interface{})
			inputSchema := toolMap["input_schema"].(map[string]interface{})

			toolParam := anthropic.ToolParam{
				Name:        toolMap["name"].(string),
				Description: anthropic.String(toolMap["description"].(string)),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: inputSchema["properties"],
				},
			}

			if required, ok := inputSchema["required"]; ok {
				if reqSlice, ok := required.([]interface{}); ok {
					strSlice := make([]string, len(reqSlice))
					for i, v := range reqSlice {
						strSlice[i] = v.(string)
					}
					toolParam.InputSchema.Required = strSlice
				}
			}

			tools = append(tools, anthropic.ToolUnionParam{OfTool: &toolParam})
		}
		reqParams.Tools = tools
	}

	// Make API call
	response, err := p.client.Messages.New(ctx, reqParams)
	if err != nil {
		return nil, err
	}

	// Extract content and tool calls
	content := ""
	toolCalls := []ToolCall{}

	for _, block := range response.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			content += b.Text
		case anthropic.ToolUseBlock:
			// Parse input from JSON
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &params); err != nil {
				return nil, fmt.Errorf("failed to parse tool input: %w", err)
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:         b.ID,
				Name:       b.Name,
				Parameters: params,
			})
		}
	}

	return &LLMResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: &TokenUsage{
			InputTokens:  int(response.Usage.InputTokens),
			OutputTokens: int(response.Usage.OutputTokens),
		},
	}, nil
}
