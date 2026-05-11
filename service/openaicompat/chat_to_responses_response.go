package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// ChatCompletionsResponseToResponsesResponse converts a Chat Completions API response
// into a Responses API response.
// This is the inverse of ResponsesResponseToChatCompletionsResponse.
func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, responseId string, instructions json.RawMessage, metadata json.RawMessage) (*dto.OpenAIResponsesResponse, error) {
	if resp == nil {
		return nil, errors.New("response is nil")
	}

	output := buildResponsesOutputFromChatResponse(resp)
	usage := buildResponsesUsageFromChatUsage(resp.Usage)

	createdAt := 0
	switch v := resp.Created.(type) {
	case int:
		createdAt = v
	case int64:
		createdAt = int(v)
	case float64:
		createdAt = int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			createdAt = int(i)
		}
	}

	status := "completed"
	if resp.Error != nil {
		status = "failed"
	}

	out := &dto.OpenAIResponsesResponse{
		ID:        responseId,
		Object:    "response",
		CreatedAt: createdAt,
		Status:    json.RawMessage(fmt.Sprintf("%q", status)),
		Model:     resp.Model,
		Output:    output,
		Usage:     usage,
	}

	// Set incomplete_details based on finish reason.
	if len(resp.Choices) > 0 {
		switch resp.Choices[0].FinishReason {
		case "length":
			out.IncompleteDetails = &dto.IncompleteDetails{Reasoning: "max_output_tokens"}
		case "content_filter":
			out.IncompleteDetails = &dto.IncompleteDetails{Reasoning: "content_filter"}
		}
	}

	// Echo back instructions and metadata from the original Responses API request.
	out.Instructions = instructions
	out.Metadata = metadata

	return out, nil
}

// buildResponsesOutputFromChatResponse converts Chat Completions choices
// into Responses API output items.
func buildResponsesOutputFromChatResponse(resp *dto.OpenAITextResponse) []dto.ResponsesOutput {
	if len(resp.Choices) == 0 {
		return nil
	}

	choice := resp.Choices[0]
	msg := choice.Message

	var output []dto.ResponsesOutput

	// Text message output.
	if msg.Content != nil {
		content := buildResponsesContentFromChatContent(msg)
		if len(content) > 0 {
			output = append(output, dto.ResponsesOutput{
				Type:    "message",
				ID:      "msg_" + common.GetRandomString(8),
				Role:    "assistant",
				Status:  "completed",
				Content: content,
			})
		}
	}

	// Tool call outputs.
	toolCalls := msg.ParseToolCalls()
	for _, tc := range toolCalls {
		callId := strings.TrimSpace(tc.ID)
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			continue
		}

		var argsRaw json.RawMessage
		argsStr := tc.Function.Arguments
		if argsStr != "" {
			// Validate that arguments is valid JSON; if not, wrap as string.
			if common.GetJsonType(json.RawMessage(argsStr)) != "" {
				argsRaw = json.RawMessage(argsStr)
			} else {
				argsRaw, _ = common.Marshal(argsStr)
			}
		}

		output = append(output, dto.ResponsesOutput{
			Type:      "function_call",
			ID:        callId,
			CallId:    callId,
			Name:      name,
			Status:    "completed",
			Arguments: argsRaw,
		})
	}

	return output
}

// buildResponsesContentFromChatContent converts Chat Completions message content
// into Responses API output content parts.
func buildResponsesContentFromChatContent(msg dto.Message) []dto.ResponsesOutputContent {
	if msg.Content == nil {
		return nil
	}

	// String content → single output_text part.
	if msg.IsStringContent() {
		text := strings.TrimSpace(msg.StringContent())
		if text == "" {
			return nil
		}
		return []dto.ResponsesOutputContent{
			{Type: "output_text", Text: text},
		}
	}

	// Array content → convert each MediaContent part.
	parts := msg.ParseContent()
	result := make([]dto.ResponsesOutputContent, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case dto.ContentTypeText:
			if strings.TrimSpace(part.Text) != "" {
				result = append(result, dto.ResponsesOutputContent{
					Type: "output_text",
					Text: part.Text,
				})
			}
		}
	}
	return result
}

// buildResponsesUsageFromChatUsage converts Chat Completions Usage
// into a format suitable for Responses API.
// The dto.Usage struct already carries both sets of field names
// (PromptTokens/InputTokens, CompletionTokens/OutputTokens),
// so we copy values into both.
func buildResponsesUsageFromChatUsage(chatUsage dto.Usage) *dto.Usage {
	usage := &dto.Usage{}

	// PromptTokens → InputTokens (and vice versa).
	if chatUsage.InputTokens != 0 {
		usage.InputTokens = chatUsage.InputTokens
		usage.PromptTokens = chatUsage.InputTokens
	} else if chatUsage.PromptTokens != 0 {
		usage.InputTokens = chatUsage.PromptTokens
		usage.PromptTokens = chatUsage.PromptTokens
	}

	// CompletionTokens → OutputTokens (and vice versa).
	if chatUsage.OutputTokens != 0 {
		usage.OutputTokens = chatUsage.OutputTokens
		usage.CompletionTokens = chatUsage.OutputTokens
	} else if chatUsage.CompletionTokens != 0 {
		usage.OutputTokens = chatUsage.CompletionTokens
		usage.CompletionTokens = chatUsage.CompletionTokens
	}

	// TotalTokens.
	if chatUsage.TotalTokens != 0 {
		usage.TotalTokens = chatUsage.TotalTokens
	} else {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// Token details.
	if chatUsage.InputTokensDetails != nil {
		usage.InputTokensDetails = chatUsage.InputTokensDetails
		usage.PromptTokensDetails = chatUsage.PromptTokensDetails
	} else if chatUsage.PromptTokensDetails.CachedTokens != 0 ||
		chatUsage.PromptTokensDetails.ImageTokens != 0 ||
		chatUsage.PromptTokensDetails.AudioTokens != 0 {
		usage.PromptTokensDetails = chatUsage.PromptTokensDetails
	}

	if chatUsage.CompletionTokenDetails.ReasoningTokens != 0 {
		usage.CompletionTokenDetails = chatUsage.CompletionTokenDetails
	}

	return usage
}
