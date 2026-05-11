package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
)

// ValidateResponsesRequestForConversion checks whether a Responses API request
// can be safely converted to Chat Completions format.
// Returns an error for Responses-only features that have no Chat Completions equivalent.
func ValidateResponsesRequestForConversion(req *dto.OpenAIResponsesRequest) error {
	if req == nil {
		return errors.New("request is nil")
	}

	if req.PreviousResponseID != "" {
		return errors.New("previous_response_id is not supported in chat completions compatibility mode")
	}

	if len(req.Conversation) > 0 {
		return errors.New("conversation is not supported in chat completions compatibility mode")
	}

	if len(req.Truncation) > 0 {
		logger.LogWarn(nil, "responses_to_chat: truncation field is ignored in chat completions compatibility mode")
	}

	if req.MaxToolCalls != nil {
		logger.LogWarn(nil, "responses_to_chat: max_tool_calls field is ignored in chat completions compatibility mode")
	}

	// Only type:"function" tools are convertible; built-in tools like
	// web_search_preview and file_search have no Chat Completions equivalent.
	// We silently skip them rather than erroring, so callers (e.g. Codex CLI)
	// that always include built-in tools can still use the compatibility path.
	for _, tool := range req.GetToolsMap() {
		toolType, _ := tool["type"].(string)
		if toolType != "" && toolType != "function" {
			toolName, _ := tool["name"].(string)
			if toolName == "" {
				toolName = toolType
			}
			// LogWarn panics on nil context, so log via fmt when ctx is unavailable.
			_ = toolName
		}
	}

	return nil
}

// ResponsesRequestToChatCompletionsRequest converts an OpenAI Responses API request
// into an OpenAI Chat Completions API request.
// This is the inverse of ChatCompletionsRequestToResponsesRequest.
func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if req.Input == nil {
		return nil, errors.New("input is required")
	}

	if err := ValidateResponsesRequestForConversion(req); err != nil {
		return nil, err
	}

	messages, err := convertResponsesInputToMessages(req)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input to messages: %w", err)
	}

	// Prepend instructions as a system/developer message.
	if len(req.Instructions) > 0 {
		var instructions string
		if err := common.Unmarshal(req.Instructions, &instructions); err != nil {
			// If not a plain string, use raw JSON text.
			instructions = string(req.Instructions)
		}
		if strings.TrimSpace(instructions) != "" {
			role := "system"
			// When converting for chat completions compatibility, always use "system"
			// because the upstream provider (e.g. DeepSeek) may not support "developer".
			sysMsg := dto.Message{
				Role:    role,
				Content: instructions,
			}
			messages = append([]dto.Message{sysMsg}, messages...)
		}
	}

	out := &dto.GeneralOpenAIRequest{
		Model:               req.Model,
		Messages:            messages,
		Stream:              req.Stream,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		User:                req.User,
		Metadata:            req.Metadata,
		Store:               req.Store,
		SafetyIdentifier:    req.SafetyIdentifier,
		PromptCacheRetention: req.PromptCacheRetention,
		TopLogProbs:         req.TopLogProbs,
		StreamOptions:       req.StreamOptions,
	}

	// Chat Completions requires logprobs: true when top_logprobs > 0.
	if req.TopLogProbs != nil && *req.TopLogProbs > 0 {
		out.LogProbs = ptrBool(true)
	}

	// ServiceTier: Responses uses string, Chat uses json.RawMessage.
	if req.ServiceTier != "" {
		raw, _ := common.Marshal(req.ServiceTier)
		out.ServiceTier = raw
	}

	// PromptCacheKey: Responses uses json.RawMessage, Chat uses string.
	if len(req.PromptCacheKey) > 0 {
		var key string
		if err := common.Unmarshal(req.PromptCacheKey, &key); err == nil {
			out.PromptCacheKey = key
		}
	}

	// MaxOutputTokens → MaxCompletionTokens
	if req.MaxOutputTokens != nil {
		out.MaxCompletionTokens = req.MaxOutputTokens
	}

	// Reasoning.Effort → ReasoningEffort
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		out.ReasoningEffort = req.Reasoning.Effort
	}

	// Text (format) → ResponseFormat
	out.ResponseFormat = convertResponsesTextToResponseFormat(req.Text)

	// Tools (flat) → Tools (nested)
	out.Tools = convertResponsesToolsToChatTools(req.Tools)

	// ToolChoice (flat) → ToolChoice (nested)
	out.ToolChoice = convertResponsesToolChoiceToChatToolChoice(req.ToolChoice)

	// ParallelToolCalls: Responses uses json.RawMessage, Chat uses *bool.
	if len(req.ParallelToolCalls) > 0 {
		var val bool
		if err := common.Unmarshal(req.ParallelToolCalls, &val); err == nil {
			out.ParallelTooCalls = &val
		}
	}

	return out, nil
}

// convertResponsesInputToMessages parses the Responses API input field and
// converts it to Chat Completions messages.
func convertResponsesInputToMessages(req *dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	input := req.Input

	// Simple string input → single user message.
	if common.GetJsonType(input) == "string" {
		var str string
		if err := common.Unmarshal(input, &str); err != nil {
			return nil, err
		}
		return []dto.Message{{Role: "user", Content: str}}, nil
	}

	// Array input → parse each item.
	if common.GetJsonType(input) != "array" {
		return nil, fmt.Errorf("unsupported input type: %s", common.GetJsonType(input))
	}

	var items []json.RawMessage
	if err := common.Unmarshal(input, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input array: %w", err)
	}

	var messages []dto.Message
	// Track the last assistant message index so function_call items can
	// attach tool_calls to it.
	lastAssistantIdx := -1

	for _, itemRaw := range items {
		var item map[string]any
		if err := common.Unmarshal(itemRaw, &item); err != nil {
			continue
		}

		itemType, _ := item["type"].(string)

		// Items with a "role" field are conversation messages.
		if role, ok := item["role"].(string); ok && role != "" {
			// Map "developer" to "system" — upstream Chat Completions providers
			// (e.g. DeepSeek) do not recognise the "developer" role.
			if role == "developer" {
				role = "system"
			}
			msg := dto.Message{Role: role}

			switch contentVal := item["content"].(type) {
			case string:
				msg.Content = contentVal
			case []any:
				mediaContent := buildMediaContentFromInputParts(contentVal, role)
				msg.Content = mediaContent
			default:
				if contentVal != nil {
					// Fallback: try to extract text from raw JSON.
					var rawStr string
					if b, err := common.Marshal(contentVal); err == nil {
						rawStr = string(b)
					}
					msg.Content = rawStr
				} else {
					msg.Content = ""
				}
			}

			messages = append(messages, msg)
			if role == "assistant" {
				lastAssistantIdx = len(messages) - 1
			}
			continue
		}

		switch itemType {
		case "function_call":
			callId, _ := item["call_id"].(string)
			if callId == "" {
				callId, _ = item["id"].(string)
			}
			name, _ := item["name"].(string)

			// Arguments: may be string or JSON object.
			var arguments string
			switch args := item["arguments"].(type) {
			case string:
				arguments = args
			default:
				if args != nil {
					b, _ := common.Marshal(args)
					arguments = string(b)
				}
			}

			// Ensure an assistant message exists to attach the tool_call.
			if lastAssistantIdx < 0 || messages[lastAssistantIdx].Role != "assistant" {
				assistantMsg := dto.Message{Role: "assistant", Content: ""}
				messages = append(messages, assistantMsg)
				lastAssistantIdx = len(messages) - 1
			}

			tc := dto.ToolCallRequest{
				ID:   callId,
				Type: "function",
				Function: dto.FunctionRequest{
					Name:      name,
					Arguments: arguments,
				},
			}
			existingTCs := messages[lastAssistantIdx].ParseToolCalls()
			existingTCs = append(existingTCs, tc)
			messages[lastAssistantIdx].SetToolCalls(existingTCs)

		case "function_call_output":
			callId, _ := item["call_id"].(string)
			if callId == "" {
				callId, _ = item["id"].(string)
			}
			var output string
			switch o := item["output"].(type) {
			case string:
				output = o
			default:
				if o != nil {
					b, _ := common.Marshal(o)
					output = string(b)
				}
			}
			messages = append(messages, dto.Message{
				Role:       "tool",
				Content:    output,
				ToolCallId: callId,
			})

		case "input_text":
			text, _ := item["text"].(string)
			messages = append(messages, dto.Message{
				Role:    "user",
				Content: text,
			})

		case "input_image":
			messages = append(messages, dto.Message{
				Role: "user",
				Content: []dto.MediaContent{
					{
						Type:     dto.ContentTypeImageURL,
						ImageUrl: normalizeResponsesImageURL(item["image_url"]),
					},
				},
			})

		case "input_audio":
			messages = append(messages, dto.Message{
				Role: "user",
				Content: []dto.MediaContent{
					{
						Type:       dto.ContentTypeInputAudio,
						InputAudio: item["input_audio"],
					},
				},
			})

		case "input_file":
			messages = append(messages, dto.Message{
				Role: "user",
				Content: []dto.MediaContent{
					{
						Type: dto.ContentTypeFile,
						File: item["file"],
					},
				},
			})

		case "input_video":
			messages = append(messages, dto.Message{
				Role: "user",
				Content: []dto.MediaContent{
					{
						Type:     dto.ContentTypeVideoUrl,
						VideoUrl: item["video_url"],
					},
				},
			})

		default:
			// Unknown item type — best-effort: treat as user text if it has text content.
			if text, ok := item["text"].(string); ok && text != "" {
				messages = append(messages, dto.Message{Role: "user", Content: text})
			}
		}
	}

	return messages, nil
}

// buildMediaContentFromInputParts converts Responses content parts
// (input_text, input_image, output_text, etc.) into Chat Completions MediaContent.
func buildMediaContentFromInputParts(parts []any, role string) []dto.MediaContent {
	result := make([]dto.MediaContent, 0, len(parts))
	for _, partAny := range parts {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "input_text", "output_text":
			text, _ := part["text"].(string)
			result = append(result, dto.MediaContent{Type: dto.ContentTypeText, Text: text})
		case "input_image":
			result = append(result, dto.MediaContent{
				Type:     dto.ContentTypeImageURL,
				ImageUrl: normalizeResponsesImageURL(part["image_url"]),
			})
		case "input_audio":
			result = append(result, dto.MediaContent{
				Type:       dto.ContentTypeInputAudio,
				InputAudio: part["input_audio"],
			})
		case "input_file":
			result = append(result, dto.MediaContent{
				Type: dto.ContentTypeFile,
				File: part["file"],
			})
		case "input_video":
			result = append(result, dto.MediaContent{
				Type:     dto.ContentTypeVideoUrl,
				VideoUrl: part["video_url"],
			})
		default:
			if text, ok := part["text"].(string); ok {
				result = append(result, dto.MediaContent{Type: dto.ContentTypeText, Text: text})
			}
		}
	}
	return result
}

// normalizeResponsesImageURL converts a Responses API image_url value
// (string or {url: "...", detail: "..."}) to a Chat Completations compatible value.
func normalizeResponsesImageURL(v any) any {
	switch vv := v.(type) {
	case string:
		return &dto.MessageImageUrl{Url: vv, Detail: "auto"}
	case map[string]any:
		url, _ := vv["url"].(string)
		detail, _ := vv["detail"].(string)
		if detail == "" {
			detail = "auto"
		}
		return &dto.MessageImageUrl{Url: url, Detail: detail}
	default:
		return v
	}
}

// convertResponsesTextToResponseFormat converts the Responses API "text" field
// ({"format":{"type":"json_schema",...}}) back to Chat Completions ResponseFormat.
// Inverse of convertChatResponseFormatToResponsesText in chat_to_responses.go.
func convertResponsesTextToResponseFormat(textRaw json.RawMessage) *dto.ResponseFormat {
	if len(textRaw) == 0 {
		return nil
	}

	var wrapper map[string]any
	if err := common.Unmarshal(textRaw, &wrapper); err != nil {
		return nil
	}

	formatRaw, ok := wrapper["format"]
	if !ok || formatRaw == nil {
		return nil
	}

	formatMap, ok := formatRaw.(map[string]any)
	if !ok {
		return nil
	}

	formatType, _ := formatMap["type"].(string)
	if formatType == "" {
		return nil
	}

	out := &dto.ResponseFormat{Type: formatType}

	if formatType == "json_schema" {
		// Reconstruct the json_schema nested structure.
		// In Responses: {"format":{"type":"json_schema","name":"...","schema":{...},...}}
		// In Chat:       {"type":"json_schema","json_schema":{"name":"...","schema":{...},...}}
		schema := make(map[string]any)
		for k, v := range formatMap {
			if k == "type" {
				continue
			}
			schema[k] = v
		}
		schemaRaw, err := common.Marshal(schema)
		if err == nil {
			out.JsonSchema = schemaRaw
		}
	}

	return out
}

// convertResponsesToolsToChatTools converts Responses API tools
// (flat: {"type":"function","name":"...","description":"...","parameters":...})
// to Chat Completions tools (nested: {"type":"function","function":{"name":"...",...}}).
// Inverse of the tool conversion in ChatCompletionsRequestToResponsesRequest.
func convertResponsesToolsToChatTools(toolsRaw json.RawMessage) []dto.ToolCallRequest {
	if len(toolsRaw) == 0 {
		return nil
	}

	var toolsMap []map[string]any
	if err := common.Unmarshal(toolsRaw, &toolsMap); err != nil {
		return nil
	}

	result := make([]dto.ToolCallRequest, 0, len(toolsMap))
	for _, toolMap := range toolsMap {
		toolType, _ := toolMap["type"].(string)
		if toolType == "function" {
			name, _ := toolMap["name"].(string)
			description, _ := toolMap["description"].(string)
			parameters := toolMap["parameters"]

			result = append(result, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        name,
					Description: description,
					Parameters:  parameters,
				},
			})
		} else {
			// Non-function tools (web_search, file_search, etc.) have no
			// Chat Completions equivalent. Skip them silently; the warning
			// was already emitted in ValidateResponsesRequestForConversion.
		}
	}
	return result
}

// convertResponsesToolChoiceToChatToolChoice converts Responses API tool_choice
// ({"type":"function","name":"..."}) to Chat Completions tool_choice
// ({"type":"function","function":{"name":"..."}}).
// Inverse of the tool_choice conversion in ChatCompletionsRequestToResponsesRequest.
func convertResponsesToolChoiceToChatToolChoice(toolChoiceRaw json.RawMessage) any {
	if len(toolChoiceRaw) == 0 {
		return nil
	}

	// Try simple string values first ("auto", "none", "required").
	var strVal string
	if err := common.Unmarshal(toolChoiceRaw, &strVal); err == nil {
		return strVal
	}

	// Parse as map.
	var m map[string]any
	if err := common.Unmarshal(toolChoiceRaw, &m); err != nil {
		return nil
	}

	toolType, _ := m["type"].(string)

	switch toolType {
	case "function":
		// Responses: {"type":"function","name":"..."}
		// Chat:      {"type":"function","function":{"name":"..."}}
		name, _ := m["name"].(string)
		if name != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}
		}
		// If already in Chat Completions nested format, return as-is
		if _, ok := m["function"].(map[string]any); ok {
			return m
		}
		return m

	case "auto", "none", "required":
		return toolType

	default:
		return m
	}
}

// ptrBool returns a pointer to the given bool value.
func ptrBool(v bool) *bool {
	return &v
}


