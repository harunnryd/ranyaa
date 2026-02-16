package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIProvider implements LLMProvider for OpenAI
type OpenAIProvider struct {
	client openai.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}
}

// Provider returns the provider name
func (p *OpenAIProvider) Provider() string {
	return "openai"
}

// Call makes an API call to OpenAI
func (p *OpenAIProvider) Call(ctx context.Context, request LLMRequest) (*LLMResponse, error) {
	// Convert messages to OpenAI format
	messages := []openai.ChatCompletionMessageParamUnion{}

	// Add system message if provided
	if request.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(request.SystemPrompt))
	}

	for _, msg := range request.Messages {
		if msg.Role == "system" {
			continue // Already handled above
		}

		switch msg.Role {
		case "user":
			messages = append(messages, openai.UserMessage(msg.Content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls - need to construct manually
				toolCalls := []openai.ChatCompletionMessageToolCall{}
				for _, tc := range msg.ToolCalls {
					// Marshal parameters to JSON string
					paramsJSON, err := json.Marshal(tc.Parameters)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal tool parameters: %w", err)
					}

					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCall{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunction{
							Name:      tc.Name,
							Arguments: string(paramsJSON),
						},
					})
				}

				// Create assistant message with tool calls using ToParam
				assistantMsg := openai.ChatCompletionMessage{
					Role:      "assistant",
					Content:   msg.Content,
					ToolCalls: toolCalls,
				}
				messages = append(messages, assistantMsg.ToParam())
			} else {
				messages = append(messages, openai.AssistantMessage(msg.Content))
			}
		case "tool":
			messages = append(messages, openai.ToolMessage(msg.ToolCallID, msg.Content))
		}
	}

	// Build request parameters
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(request.Model),
		Messages: messages,
	}

	if request.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(request.MaxTokens))
	}

	if request.Temperature > 0 {
		params.Temperature = openai.Float(request.Temperature)
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools := []openai.ChatCompletionToolParam{}
		for _, tool := range request.Tools {
			toolMap := tool.(map[string]interface{})
			tools = append(tools, openai.ChatCompletionToolParam{
				Type: "function",
				Function: openai.FunctionDefinitionParam{
					Name:        toolMap["name"].(string),
					Description: openai.String(toolMap["description"].(string)),
					Parameters:  openai.FunctionParameters(toolMap["input_schema"].(map[string]interface{})),
				},
			})
		}
		params.Tools = tools
	}

	// Make API call
	response, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned")
	}

	choice := response.Choices[0]

	// Extract content
	content := choice.Message.Content

	// Extract tool calls
	toolCalls := []ToolCall{}
	if len(choice.Message.ToolCalls) > 0 {
		for _, tc := range choice.Message.ToolCalls {
			// Parse arguments from JSON string
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
			}

			toolCalls = append(toolCalls, ToolCall{
				ID:         tc.ID,
				Name:       tc.Function.Name,
				Parameters: params,
			})
		}
	}

	return &LLMResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: &TokenUsage{
			InputTokens:  int(response.Usage.PromptTokens),
			OutputTokens: int(response.Usage.CompletionTokens),
		},
	}, nil
}
