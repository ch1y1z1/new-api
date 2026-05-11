package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ChatCompletionsResponseToResponsesResponse tests ---

func TestChatCompletionsResponseToResponsesResponse_NilResponse(t *testing.T) {
	_, err := ChatCompletionsResponseToResponsesResponse(nil, "resp_123", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestChatCompletionsResponseToResponsesResponse_BasicText(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:    "chatcmpl-abc",
		Model: "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_test123", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "resp_test123", resp.ID)
	assert.Equal(t, "response", resp.Object)
	assert.Equal(t, "gpt-4o", resp.Model)
	assert.Equal(t, `"completed"`, string(resp.Status))
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 8, resp.Usage.OutputTokens)
	assert.Equal(t, 18, resp.Usage.TotalTokens)

	// Output should have one "message" item with "output_text" content
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "assistant", resp.Output[0].Role)
	require.Len(t, resp.Output[0].Content, 1)
	assert.Equal(t, "output_text", resp.Output[0].Content[0].Type)
	assert.Equal(t, "Hello! How can I help you?", resp.Output[0].Content[0].Text)
}

func TestChatCompletionsResponseToResponsesResponse_WithToolCalls(t *testing.T) {
	toolCallsJSON := `[{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Tokyo\"}"}}]`
	chatResp := &dto.OpenAITextResponse{
		Id:     "chatcmpl-def",
		Model:  "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:      "assistant",
					Content:   "",
					ToolCalls: json.RawMessage(toolCallsJSON),
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     20,
			CompletionTokens: 15,
			TotalTokens:      35,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_tool123", nil, nil)
	require.NoError(t, err)

	// Should have one function_call output item
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "function_call", resp.Output[0].Type)
	assert.Equal(t, "call_abc", resp.Output[0].CallId)
	assert.Equal(t, "get_weather", resp.Output[0].Name)

	// Arguments should be valid JSON
	var args map[string]any
	err = json.Unmarshal(resp.Output[0].Arguments, &args)
	require.NoError(t, err)
	assert.Equal(t, "Tokyo", args["city"])
}

func TestChatCompletionsResponseToResponsesResponse_TextAndToolCalls(t *testing.T) {
	toolCallsJSON := `[{"id":"call_xyz","type":"function","function":{"name":"search","arguments":"{\"q\":\"weather\"}"}}]`
	chatResp := &dto.OpenAITextResponse{
		Id:     "chatcmpl-mixed",
		Model:  "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:      "assistant",
					Content:   "Let me search for that.",
					ToolCalls: json.RawMessage(toolCallsJSON),
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     30,
			CompletionTokens: 20,
			TotalTokens:      50,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_mixed", nil, nil)
	require.NoError(t, err)

	// Should have message + function_call
	require.Len(t, resp.Output, 2)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "function_call", resp.Output[1].Type)
}

func TestChatCompletionsResponseToResponsesResponse_EmptyChoices(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl-empty",
		Model:   "gpt-4o",
		Choices: []dto.OpenAITextResponseChoice{},
		Usage: dto.Usage{
			PromptTokens:     5,
			CompletionTokens: 0,
			TotalTokens:      5,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_empty", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, resp.Output)
}

func TestChatCompletionsResponseToResponsesResponse_ErrorStatus(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:     "chatcmpl-err",
		Model:  "gpt-4o",
		Error:  map[string]any{"message": "rate limit exceeded"},
		Choices: []dto.OpenAITextResponseChoice{},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_err", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, `"failed"`, string(resp.Status))
}

func TestChatCompletionsResponseToResponsesResponse_CreatedInt(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl-ts",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message: dto.Message{Role: "assistant", Content: "hi"},
			},
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_ts", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1700000000, resp.CreatedAt)
}

func TestChatCompletionsResponseToResponsesResponse_CreatedFloat64(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl-tsf",
		Model:   "gpt-4o",
		Created: float64(1700000000),
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message: dto.Message{Role: "assistant", Content: "hi"},
			},
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_tsf", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 1700000000, resp.CreatedAt)
}

// --- Usage mapping tests ---

func TestChatCompletionsResponseToResponsesResponse_UsageMapping(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:     "chatcmpl-usage",
		Model:  "gpt-4o",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message: dto.Message{Role: "assistant", Content: "test"},
			},
		},
		Usage: dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_usage", nil, nil)
	require.NoError(t, err)

	// Both sets of fields should be populated
	assert.Equal(t, 100, resp.Usage.PromptTokens)
	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.CompletionTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 150, resp.Usage.TotalTokens)
}

func TestChatCompletionsResponseToResponsesResponse_UsageWithInputOutputTokens(t *testing.T) {
	chatResp := &dto.OpenAITextResponse{
		Id:     "chatcmpl-io",
		Model:  "gpt-4o",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message: dto.Message{Role: "assistant", Content: "test"},
			},
		},
		Usage: dto.Usage{
			InputTokens:  200,
			OutputTokens: 80,
		},
	}

	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_io", nil, nil)
	require.NoError(t, err)

	// InputTokens/OutputTokens should take priority when present
	assert.Equal(t, 200, resp.Usage.InputTokens)
	assert.Equal(t, 200, resp.Usage.PromptTokens)
	assert.Equal(t, 80, resp.Usage.OutputTokens)
	assert.Equal(t, 80, resp.Usage.CompletionTokens)
}

// --- Content conversion tests ---

func TestBuildResponsesContentFromChatContent_StringContent(t *testing.T) {
	msg := dto.Message{
		Role:    "assistant",
		Content: "Simple text response",
	}
	content := buildResponsesContentFromChatContent(msg)
	require.Len(t, content, 1)
	assert.Equal(t, "output_text", content[0].Type)
	assert.Equal(t, "Simple text response", content[0].Text)
}

func TestBuildResponsesContentFromChatContent_EmptyContent(t *testing.T) {
	msg := dto.Message{
		Role:    "assistant",
		Content: "",
	}
	content := buildResponsesContentFromChatContent(msg)
	// Empty string is trimmed to "" => nil
	assert.Nil(t, content)
}

func TestBuildResponsesContentFromChatContent_NilContent(t *testing.T) {
	msg := dto.Message{
		Role:    "assistant",
		Content: nil,
	}
	content := buildResponsesContentFromChatContent(msg)
	assert.Nil(t, content)
}

// --- Roundtrip: Responses request → Chat request → Chat response → Responses response ---

func TestRoundtrip_SimpleConversation(t *testing.T) {
	// 1. Start with a Responses API request
	req := &dto.OpenAIResponsesRequest{
		Model:        "gpt-4o",
		Input:        json.RawMessage(`"What is 2+2?"`),
		Instructions: json.RawMessage(`"You are a math tutor."`),
	}

	// 2. Convert to Chat Completions request
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", chatReq.Model)
	require.Len(t, chatReq.Messages, 2) // system + user

	// 3. Simulate a Chat Completions response
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl-rt",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      dto.Message{Role: "assistant", Content: "2+2 equals 4."},
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     15,
			CompletionTokens: 10,
			TotalTokens:      25,
		},
	}

	// 4. Convert back to Responses API response
	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_roundtrip", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "resp_roundtrip", resp.ID)
	assert.Equal(t, `"completed"`, string(resp.Status))
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "message", resp.Output[0].Type)
	require.Len(t, resp.Output[0].Content, 1)
	assert.Equal(t, "output_text", resp.Output[0].Content[0].Type)
	assert.Equal(t, "2+2 equals 4.", resp.Output[0].Content[0].Text)
}

func TestRoundtrip_ToolCallConversation(t *testing.T) {
	// 1. Responses request with tool call flow
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"type":"input_text","text":"Weather in Paris?"},
			{"type":"function_call","id":"call_w1","name":"get_weather","arguments":"{\"city\":\"Paris\"}"},
			{"type":"function_call_output","call_id":"call_w1","output":"Rainy, 12°C"}
		]`),
		Tools: json.RawMessage(`[{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}]`),
	}

	// 2. Convert to Chat Completions request
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)

	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Equal(t, "tool", chatReq.Messages[2].Role)

	// 3. Simulate Chat Completions response with tool call
	toolCallsJSON := `[{"id":"call_w2","type":"function","function":{"name":"get_forecast","arguments":"{\"city\":\"Paris\"}"}}]`
	chatResp := &dto.OpenAITextResponse{
		Id:      "chatcmpl-rt2",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      dto.Message{Role: "assistant", Content: "Let me get the forecast.", ToolCalls: json.RawMessage(toolCallsJSON)},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	}

	// 4. Convert back
	resp, err := ChatCompletionsResponseToResponsesResponse(chatResp, "resp_rt2", nil, nil)
	require.NoError(t, err)
	require.Len(t, resp.Output, 2) // message + function_call
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "function_call", resp.Output[1].Type)
	assert.Equal(t, "get_forecast", resp.Output[1].Name)
}
