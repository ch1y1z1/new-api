package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ValidateResponsesRequestForConversion tests ---

func TestValidateResponsesRequestForConversion_NilRequest(t *testing.T) {
	err := ValidateResponsesRequestForConversion(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestValidateResponsesRequestForConversion_PreviousResponseID(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		PreviousResponseID: "resp_abc123",
	}
	err := ValidateResponsesRequestForConversion(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "previous_response_id")
}

func TestValidateResponsesRequestForConversion_Conversation(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Conversation: json.RawMessage(`[{"type":"message","role":"user","content":"hi"}]`),
	}
	err := ValidateResponsesRequestForConversion(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversation")
}

func TestValidateResponsesRequestForConversion_BuiltInTool(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Tools: json.RawMessage(`[{"type":"web_search_preview"}]`),
	}
	// Built-in tools are now silently skipped (warned) instead of rejected,
	// so that callers like Codex CLI that always include them can still work.
	err := ValidateResponsesRequestForConversion(req)
	assert.NoError(t, err)
}

func TestValidateResponsesRequestForConversion_FunctionToolOK(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Tools: json.RawMessage(`[{"type":"function","name":"get_weather","description":"Get weather","parameters":{}}]`),
	}
	err := ValidateResponsesRequestForConversion(req)
	assert.NoError(t, err)
}

func TestValidateResponsesRequestForConversion_NoToolsOK(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`"hello"`),
	}
	err := ValidateResponsesRequestForConversion(req)
	assert.NoError(t, err)
}

// --- ResponsesRequestToChatCompletionsRequest tests ---

func TestResponsesRequestToChatCompletionsRequest_NilRequest(t *testing.T) {
	_, err := ResponsesRequestToChatCompletionsRequest(nil)
	assert.Error(t, err)
}

func TestResponsesRequestToChatCompletionsRequest_NoModel(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{}
	_, err := ResponsesRequestToChatCompletionsRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestResponsesRequestToChatCompletionsRequest_NoInput(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
	}
	_, err := ResponsesRequestToChatCompletionsRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input")
}

func TestResponsesRequestToChatCompletionsRequest_StringInput(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`"Hello, world!"`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	assert.Equal(t, "gpt-4o", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "Hello, world!", chatReq.Messages[0].Content)
}

func TestResponsesRequestToChatCompletionsRequest_ArrayInputWithConversation(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"type":"input_text","text":"What's the weather?"},
			{"type":"message","role":"assistant","content":"I can help with that."},
			{"type":"message","role":"user","content":"In Tokyo"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	require.Len(t, chatReq.Messages, 3)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "What's the weather?", chatReq.Messages[0].Content)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Equal(t, "In Tokyo", chatReq.Messages[2].Content)
}

func TestResponsesRequestToChatCompletionsRequest_Instructions(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:        "gpt-4o",
		Input:        json.RawMessage(`"Hello"`),
		Instructions: json.RawMessage(`"You are a helpful assistant."`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq)

	// System message should be prepended
	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", chatReq.Messages[0].Content)
	assert.Equal(t, "user", chatReq.Messages[1].Role)
}

func TestResponsesRequestToChatCompletionsRequest_DeveloperRole(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:        "o3",
		Input:        json.RawMessage(`"Solve this"`),
		Instructions: json.RawMessage(`"Think step by step."`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
}

func TestResponsesRequestToChatCompletionsRequest_MaxOutputTokens(t *testing.T) {
	maxTokens := uint(1024)
	req := &dto.OpenAIResponsesRequest{
		Model:           "gpt-4o",
		Input:           json.RawMessage(`"hi"`),
		MaxOutputTokens: &maxTokens,
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	assert.Equal(t, uint(1024), *chatReq.MaxCompletionTokens)
}

func TestResponsesRequestToChatCompletionsRequest_ReasoningEffort(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:  "o3",
		Input:  json.RawMessage(`"think"`),
		Reasoning: &dto.Reasoning{Effort: "high"},
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "high", chatReq.ReasoningEffort)
}

func TestResponsesRequestToChatCompletionsRequest_TemperatureAndTopP(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &dto.OpenAIResponsesRequest{
		Model:       "gpt-4o",
		Input:       json.RawMessage(`"hi"`),
		Temperature: &temp,
		TopP:        &topP,
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq.Temperature)
	assert.Equal(t, 0.7, *chatReq.Temperature)
	require.NotNil(t, chatReq.TopP)
	assert.Equal(t, 0.9, *chatReq.TopP)
}

func TestResponsesRequestToChatCompletionsRequest_FunctionTools(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`"hi"`),
		Tools: json.RawMessage(`[
			{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	assert.Equal(t, "function", chatReq.Tools[0].Type)
	assert.Equal(t, "get_weather", chatReq.Tools[0].Function.Name)
	assert.Equal(t, "Get weather", chatReq.Tools[0].Function.Description)
}

func TestResponsesRequestToChatCompletionsRequest_ToolChoiceAuto(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-4o",
		Input:      json.RawMessage(`"hi"`),
		ToolChoice: json.RawMessage(`"auto"`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	assert.Equal(t, "auto", chatReq.ToolChoice)
}

func TestResponsesRequestToChatCompletionsRequest_ToolChoiceFunction(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:      "gpt-4o",
		Input:      json.RawMessage(`"hi"`),
		ToolChoice: json.RawMessage(`{"type":"function","name":"get_weather"}`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	// Should be nested: {"type":"function","function":{"name":"get_weather"}}
	choiceMap, ok := chatReq.ToolChoice.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function", choiceMap["type"])
	fnMap, ok := choiceMap["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "get_weather", fnMap["name"])
}

func TestResponsesRequestToChatCompletionsRequest_ResponseFormatJsonSchema(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`"hi"`),
		Text: json.RawMessage(`{
			"format": {
				"type": "json_schema",
				"name": "my_schema",
				"schema": {"type":"object","properties":{"result":{"type":"string"}}}
			}
		}`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq.ResponseFormat)
	assert.Equal(t, "json_schema", chatReq.ResponseFormat.Type)
	assert.NotNil(t, chatReq.ResponseFormat.JsonSchema)
}

func TestResponsesRequestToChatCompletionsRequest_FunctionCallAndOutput(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"type":"input_text","text":"What's the weather?"},
			{"type":"function_call","id":"call_123","name":"get_weather","arguments":"{\"city\":\"Tokyo\"}"},
			{"type":"function_call_output","call_id":"call_123","output":"Sunny, 22°C"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	// Expect: user msg, assistant msg with tool_call, tool msg
	require.Len(t, chatReq.Messages, 3)

	// First: user message
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "What's the weather?", chatReq.Messages[0].Content)

	// Second: assistant with tool_calls
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	toolCalls := chatReq.Messages[1].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_123", toolCalls[0].ID)
	assert.Equal(t, "get_weather", toolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"Tokyo"}`, toolCalls[0].Function.Arguments)

	// Third: tool message
	assert.Equal(t, "tool", chatReq.Messages[2].Role)
	assert.Equal(t, "Sunny, 22°C", chatReq.Messages[2].Content)
	assert.Equal(t, "call_123", chatReq.Messages[2].ToolCallId)
}

func TestResponsesRequestToChatCompletionsRequest_InputImage(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"type":"input_image","image_url":"https://example.com/img.png"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	// Content should be []MediaContent with an image_url part
	mediaContent, ok := chatReq.Messages[0].Content.([]dto.MediaContent)
	require.True(t, ok, "expected Content to be []dto.MediaContent")
	require.Len(t, mediaContent, 1)
	assert.Equal(t, dto.ContentTypeImageURL, mediaContent[0].Type)
}

func TestResponsesRequestToChatCompletionsRequest_MultimodalContentParts(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{
				"type":"message",
				"role":"user",
				"content":[
					{"type":"input_text","text":"Describe this image"},
					{"type":"input_image","image_url":"https://example.com/img.png"}
				]
			}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)

	mediaContent, ok := chatReq.Messages[0].Content.([]dto.MediaContent)
	require.True(t, ok, "expected Content to be []dto.MediaContent")
	require.Len(t, mediaContent, 2)
	assert.Equal(t, dto.ContentTypeText, mediaContent[0].Type)
	assert.Equal(t, "Describe this image", mediaContent[0].Text)
	assert.Equal(t, dto.ContentTypeImageURL, mediaContent[1].Type)
}

// --- Roundtrip test: Responses → Chat → verify key fields survive ---

func TestResponsesRequestToChatCompletionsRequest_RoundtripBasic(t *testing.T) {
	temp := 0.5
	maxTokens := uint(2048)
	streamTrue := true

	req := &dto.OpenAIResponsesRequest{
		Model:           "gpt-4o",
		Input:           json.RawMessage(`"Tell me a joke"`),
		Instructions:    json.RawMessage(`"Be funny"`),
		MaxOutputTokens: &maxTokens,
		Temperature:     &temp,
		Stream:          &streamTrue,
		Tools: json.RawMessage(`[
			{"type":"function","name":"search","description":"Search the web","parameters":{"type":"object"}}
		]`),
		ToolChoice: json.RawMessage(`"auto"`),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o", chatReq.Model)
	assert.True(t, *chatReq.Stream)
	require.NotNil(t, chatReq.Temperature)
	assert.InDelta(t, 0.5, *chatReq.Temperature, 0.001)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	assert.Equal(t, uint(2048), *chatReq.MaxCompletionTokens)
	require.Len(t, chatReq.Tools, 1)
	assert.Equal(t, "search", chatReq.Tools[0].Function.Name)
	assert.Equal(t, "auto", chatReq.ToolChoice)

	// Should have system + user = 2 messages
	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "user", chatReq.Messages[1].Role)
}

// --- normalizeResponsesImageURL tests ---

func TestNormalizeResponsesImageURL_String(t *testing.T) {
	result := normalizeResponsesImageURL("https://example.com/img.png")
	imgUrl, ok := result.(*dto.MessageImageUrl)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/img.png", imgUrl.Url)
	assert.Equal(t, "auto", imgUrl.Detail)
}

func TestNormalizeResponsesImageURL_MapWithDetail(t *testing.T) {
	result := normalizeResponsesImageURL(map[string]any{
		"url":    "https://example.com/img.png",
		"detail": "high",
	})
	imgUrl, ok := result.(*dto.MessageImageUrl)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/img.png", imgUrl.Url)
	assert.Equal(t, "high", imgUrl.Detail)
}

// --- convertResponsesTextToResponseFormat tests ---

func TestConvertResponsesTextToResponseFormat_Empty(t *testing.T) {
	assert.Nil(t, convertResponsesTextToResponseFormat(nil))
	assert.Nil(t, convertResponsesTextToResponseFormat(json.RawMessage(``)))
}

func TestConvertResponsesTextToResponseFormat_JsonSchema(t *testing.T) {
	text := json.RawMessage(`{
		"format": {
			"type": "json_schema",
			"name": "test_schema",
			"schema": {"type": "object", "properties": {"x": {"type": "number"}}},
			"strict": true
		}
	}`)
	result := convertResponsesTextToResponseFormat(text)
	require.NotNil(t, result)
	assert.Equal(t, "json_schema", result.Type)
	assert.NotNil(t, result.JsonSchema)

	// Verify json_schema nested structure
	var schema map[string]any
	err := common.Unmarshal(result.JsonSchema, &schema)
	require.NoError(t, err)
	assert.Equal(t, "test_schema", schema["name"])
	assert.NotNil(t, schema["schema"])
}

// --- convertResponsesToolChoiceToChatToolChoice tests ---

func TestConvertResponsesToolChoiceToChatToolChoice_Auto(t *testing.T) {
	result := convertResponsesToolChoiceToChatToolChoice(json.RawMessage(`"auto"`))
	assert.Equal(t, "auto", result)
}

func TestConvertResponsesToolChoiceToChatToolChoice_None(t *testing.T) {
	result := convertResponsesToolChoiceToChatToolChoice(json.RawMessage(`"none"`))
	assert.Equal(t, "none", result)
}

func TestConvertResponsesToolChoiceToChatToolChoice_Required(t *testing.T) {
	result := convertResponsesToolChoiceToChatToolChoice(json.RawMessage(`"required"`))
	assert.Equal(t, "required", result)
}

func TestConvertResponsesToolChoiceToChatToolChoice_Function(t *testing.T) {
	raw := json.RawMessage(`{"type":"function","name":"get_weather"}`)
	result := convertResponsesToolChoiceToChatToolChoice(raw)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function", m["type"])
	fn, ok := m["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "get_weather", fn["name"])
}

func TestConvertResponsesToolChoiceToChatToolChoice_Empty(t *testing.T) {
	assert.Nil(t, convertResponsesToolChoiceToChatToolChoice(nil))
	assert.Nil(t, convertResponsesToolChoiceToChatToolChoice(json.RawMessage(``)))
}

// --- New: json_object response format ---

func TestConvertResponsesTextToResponseFormat_JsonObject(t *testing.T) {
	text := json.RawMessage(`{"format":{"type":"json_object"}}`)
	result := convertResponsesTextToResponseFormat(text)
	require.NotNil(t, result)
	assert.Equal(t, "json_object", result.Type)
	assert.Nil(t, result.JsonSchema)
}

func TestConvertResponsesTextToResponseFormat_Text(t *testing.T) {
	text := json.RawMessage(`{"format":{"type":"text"}}`)
	result := convertResponsesTextToResponseFormat(text)
	assert.Nil(t, result)
}

// --- New: enforceParameterTypeObject ---

func TestEnforceParameterTypeObject_AddsType(t *testing.T) {
	params := map[string]any{
		"properties": map[string]any{"city": map[string]any{"type": "string"}},
	}
	result := enforceParameterTypeObject(params)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", m["type"])
}

func TestEnforceParameterTypeObject_ExistingType(t *testing.T) {
	params := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	result := enforceParameterTypeObject(params)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", m["type"])
}

func TestEnforceParameterTypeObject_NonMap(t *testing.T) {
	result := enforceParameterTypeObject("not a map")
	assert.Equal(t, "not a map", result)
}

func TestEnforceParameterTypeObject_Nil(t *testing.T) {
	result := enforceParameterTypeObject(nil)
	assert.Nil(t, result)
}

// --- P1: web_search_preview → web_search_options ---

func TestConvertResponsesToolsToChatTools_WebSearchPreview(t *testing.T) {
	tools := json.RawMessage(`[
		{"type":"web_search_preview","search_context_size":"high","user_location":{"type":"approximate","city":"Tokyo"}},
		{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object"}}
	]`)
	chatTools, webSearchOpts := convertResponsesToolsToChatTools(tools)
	require.Len(t, chatTools, 1)
	assert.Equal(t, "get_weather", chatTools[0].Function.Name)
	require.NotNil(t, webSearchOpts)
	assert.Equal(t, "high", webSearchOpts.SearchContextSize)
	assert.NotNil(t, webSearchOpts.UserLocation)
}

func TestConvertResponsesToolsToChatTools_WebSearchPreviewDefault(t *testing.T) {
	tools := json.RawMessage(`[{"type":"web_search_preview"}]`)
	chatTools, webSearchOpts := convertResponsesToolsToChatTools(tools)
	assert.Empty(t, chatTools)
	require.NotNil(t, webSearchOpts)
	assert.Equal(t, "medium", webSearchOpts.SearchContextSize)
}

func TestResponsesRequestToChatCompletionsRequest_WebSearchOptions(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`"latest news"`),
		Tools: json.RawMessage(`[{"type":"web_search_preview","search_context_size":"low"}]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, chatReq.WebSearchOptions)
	assert.Equal(t, "low", chatReq.WebSearchOptions.SearchContextSize)
}

// --- P3: web_search_call input item ---

func TestResponsesRequestToChatCompletionsRequest_WebSearchCallInput(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"type":"input_text","text":"Search for weather"},
			{"type":"web_search_call","id":"ws_123","status":"completed","query":"weather in Tokyo"},
			{"type":"message","role":"assistant","content":"The weather in Tokyo is sunny."}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	// Should have: user, tool (web_search_call), assistant
	require.Len(t, chatReq.Messages, 3)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "tool", chatReq.Messages[1].Role)
	assert.Equal(t, "ws_123", chatReq.Messages[1].ToolCallId)
	assert.Contains(t, chatReq.Messages[1].Content, "weather in Tokyo")
	assert.Equal(t, "assistant", chatReq.Messages[2].Role)
}

func TestResponsesRequestToChatCompletionsRequest_ReasoningInputItem(t *testing.T) {
	t.Parallel()
	// When a "reasoning" input item precedes an assistant message,
	// its content should be set as ReasoningContent on the assistant message.
	// The reasoning item itself does NOT become a separate message.
	req := &dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-flash",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":"What is 1+1?"},
			{"type":"reasoning","id":"rs_abc123","content":[{"type":"reasoning_text","text":"Simple addition: 1+1=2"}]},
			{"type":"message","role":"assistant","content":"1+1 equals 2."},
			{"type":"message","role":"user","content":"What is 2+2?"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.NotNil(t, chatReq.Messages[1].ReasoningContent)
	assert.Equal(t, "Simple addition: 1+1=2", *chatReq.Messages[1].ReasoningContent)
	assert.Equal(t, "user", chatReq.Messages[2].Role)
}

func TestResponsesRequestToChatCompletionsRequest_ReasoningSummaryFallback(t *testing.T) {
	t.Parallel()
	// When reasoning has no "content" but has "summary", use summary text.
	req := &dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-flash",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":"Hello"},
			{"type":"reasoning","id":"rs_xyz","summary":[{"type":"summary_text","text":"Greeting the user"}]},
			{"type":"message","role":"assistant","content":"Hi there!"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	require.NotNil(t, chatReq.Messages[1].ReasoningContent)
	assert.Equal(t, "Greeting the user", *chatReq.Messages[1].ReasoningContent)
}

func TestResponsesRequestToChatCompletionsRequest_NoReasoningContent(t *testing.T) {
	t.Parallel()
	// When there is no reasoning item, ReasoningContent should be nil.
	req := &dto.OpenAIResponsesRequest{
		Model: "deepseek-v4-flash",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":"Hello"},
			{"type":"message","role":"assistant","content":"Hi there!"}
		]`),
	}
	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)
	assert.Nil(t, chatReq.Messages[1].ReasoningContent)
}
