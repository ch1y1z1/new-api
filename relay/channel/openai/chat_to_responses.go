package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// OaiChatToResponsesHandler reads a non-streaming Chat Completions response,
// converts it to Responses API format, and writes it to the client.
func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responseId := generateResponseID(c)
	responsesResp, err := service.ChatCompletionsResponseToResponsesResponse(&chatResp, responseId, info.OriginalResponsesInstructions, info.OriginalResponsesMetadata)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	usage := &chatResp.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := extractChatTextContent(&chatResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = usage
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

// OaiChatToResponsesStreamHandler reads a streaming Chat Completions response,
// converts each SSE chunk to Responses API events in real-time,
// and writes them to the client.
func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := generateResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

	var (
		usage       = &dto.Usage{}
		usageText   strings.Builder
		streamErr   *types.NewAPIError
		sentCreated bool
		sentStop    bool
		sawToolCall bool

		// Assistant message item tracking.
		messageItemID string
		outputIndex   int
		contentIndex  int

		// Tool call tracking.
		toolCallItemIDs    = make(map[int]string) // chat index → responses item ID
		toolCallCallIDs    = make(map[int]string) // chat index → call_id from upstream
		toolCallNames      = make(map[int]string) // chat index → function name
		toolCallAccumIndex int
		toolCallArgs       = make(map[int]string) // chat index → accumulated arguments
		outputText         strings.Builder
	)

	// sendEvent sends a Responses API SSE event using the event+data format.
	sendEvent := func(eventType string, payload any) bool {
		data, err := common.Marshal(payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
		return true
	}

	ensureCreated := func() bool {
		if sentCreated {
			return true
		}
		if !sendEvent("response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":         responseId,
				"object":     "response",
				"created_at": createAt,
				"status":     "in_progress",
				"model":      model,
				"output":     []any{},
			},
		}) {
			return false
		}
		sentCreated = true
		return true
	}

	ensureMessageItem := func() bool {
		if messageItemID != "" {
			return true
		}
		if !ensureCreated() {
			return false
		}
		messageItemID = "msg_" + common.GetRandomString(8)
		oi := outputIndex
		if !sendEvent("response.output_item.added", map[string]any{
			"type":        "response.output_item.added",
			"output_index": oi,
			"item": map[string]any{
				"type":    "message",
				"id":      messageItemID,
				"role":    "assistant",
				"content": []any{},
				"status":  "in_progress",
			},
		}) {
			return false
		}
		ci := contentIndex
		if !sendEvent("response.content_part.added", map[string]any{
			"type":         "response.content_part.added",
			"output_index": oi,
			"content_index": ci,
			"part": map[string]any{
				"type": "output_text",
				"text": "",
			},
		}) {
			return false
		}
		return true
	}

	sendTextDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		if !ensureCreated() || !ensureMessageItem() {
			return false
		}
		outputText.WriteString(delta)
		usageText.WriteString(delta)
		oi := outputIndex
		ci := contentIndex
		return sendEvent("response.output_text.delta", map[string]any{
			"type":          "response.output_text.delta",
			"output_index":  oi,
			"content_index": ci,
			"delta":         delta,
		})
	}

	sendReasoningSummaryDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		// Map reasoning to output_text for broad compatibility.
		// A future enhancement can emit proper response.reasoning_summary_text.delta events.
		return sendTextDelta(delta)
	}

	sendToolCallItem := func(chatIndex int, callID string, name string, argsDelta string) bool {
		if !ensureCreated() {
			return false
		}

		_, exists := toolCallItemIDs[chatIndex]
		if !exists {
			itemID := "fc_" + common.GetRandomString(8)
			toolCallItemIDs[chatIndex] = itemID
			toolCallCallIDs[chatIndex] = callID
			toolCallNames[chatIndex] = name
			fcOutputIndex := outputIndex + len(toolCallItemIDs)
			if !sendEvent("response.output_item.added", map[string]any{
				"type":        "response.output_item.added",
				"output_index": fcOutputIndex,
				"item": map[string]any{
					"type":    "function_call",
					"id":      itemID,
					"call_id": callID,
					"name":    name,
					"status":  "in_progress",
				},
			}) {
				return false
			}
			sawToolCall = true
		}

		if argsDelta != "" {
			toolCallArgs[chatIndex] += argsDelta
			usageText.WriteString(argsDelta)
		}

		itemID := toolCallItemIDs[chatIndex]
		return sendEvent("response.function_call_arguments.delta", map[string]any{
			"type":    "response.function_call_arguments.delta",
			"item_id": itemID,
			"delta":   argsDelta,
		})
	}

	sendCompleted := func(finalUsage *dto.Usage) bool {
		// Close the message content part if active.
		if messageItemID != "" {
			oi := outputIndex
			ci := contentIndex
			if !sendEvent("response.output_text.done", map[string]any{
				"type":          "response.output_text.done",
				"output_index":  oi,
				"content_index": ci,
				"text":          outputText.String(),
			}) {
				return false
			}
			if !sendEvent("response.content_part.done", map[string]any{
				"type":          "response.content_part.done",
				"output_index":  oi,
				"content_index": ci,
				"part": map[string]any{
					"type": "output_text",
					"text": outputText.String(),
				},
			}) {
				return false
			}
			if !sendEvent("response.output_item.done", map[string]any{
				"type":        "response.output_item.done",
				"output_index": oi,
				"item": map[string]any{
					"type":    "message",
					"id":      messageItemID,
					"role":    "assistant",
					"content": []map[string]any{{"type": "output_text", "text": outputText.String()}},
					"status":  "completed",
				},
			}) {
				return false
			}
			messageItemID = ""
		}

		// Close function_call items.
		for chatIdx, itemID := range toolCallItemIDs {
			args := toolCallArgs[chatIdx]
			if !sendEvent("response.function_call_arguments.done", map[string]any{
				"type":      "response.function_call_arguments.done",
				"item_id":   itemID,
				"call_id":   toolCallCallIDs[chatIdx],
				"name":      toolCallNames[chatIdx],
				"arguments": args,
			}) {
				return false
			}
			fcOutputIndex := outputIndex + chatIdx + 1
			if !sendEvent("response.output_item.done", map[string]any{
				"type":        "response.output_item.done",
				"output_index": fcOutputIndex,
				"item": map[string]any{
					"type":      "function_call",
					"id":        itemID,
					"call_id":   toolCallCallIDs[chatIdx],
					"name":      toolCallNames[chatIdx],
					"status":    "completed",
					"arguments": args,
				},
			}) {
				return false
			}
		}

		// Build final output items for the completed event.
		finalOutput := []map[string]any{}
		if outputText.Len() > 0 || !sawToolCall {
			finalOutput = append(finalOutput, map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]any{{"type": "output_text", "text": outputText.String()}},
				"status":  "completed",
			})
		}
		for chatIdx, itemID := range toolCallItemIDs {
			finalOutput = append(finalOutput, map[string]any{
				"type":      "function_call",
				"id":        itemID,
				"call_id":   toolCallCallIDs[chatIdx],
				"name":      toolCallNames[chatIdx],
				"arguments": toolCallArgs[chatIdx],
				"status":    "completed",
			})
		}

		usageMap := buildUsageMap(finalUsage)

		if !sendEvent("response.completed", map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":         responseId,
				"object":     "response",
				"created_at": createAt,
				"status":     "completed",
				"model":      model,
				"output":     finalOutput,
				"usage":      usageMap,
			},
		}) {
			return false
		}
		sentStop = true
		return true
	}

	// Main loop: read Chat Completions SSE chunks from upstream.
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "chat_to_responses: failed to unmarshal stream chunk: "+err.Error())
			sr.Error(err)
			return
		}

		if chunk.Model != "" {
			model = chunk.Model
		}

		for _, choice := range chunk.Choices {
			// Role announcement — first chunk.
			if choice.Delta.Role == "assistant" {
				if !ensureCreated() {
					sr.Stop(streamErr)
					return
				}
			}

			// Text content delta.
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				if !sendTextDelta(*choice.Delta.Content) {
					sr.Stop(streamErr)
					return
				}
			}

			// Reasoning content delta.
			if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
				if !sendReasoningSummaryDelta(*choice.Delta.ReasoningContent) {
					sr.Stop(streamErr)
					return
				}
			}

			// Tool calls delta.
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				callID := tc.ID
				name := tc.Function.Name
				argsDelta := tc.Function.Arguments

				if !sendToolCallItem(idx, callID, name, argsDelta) {
					sr.Stop(streamErr)
					return
				}
				if toolCallAccumIndex <= idx {
					toolCallAccumIndex = idx + 1
				}
			}

			// Finish reason.
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				// If no text or tool call content was sent, still emit a created event.
				if !ensureCreated() {
					sr.Stop(streamErr)
					return
				}
			}
		}

		// Standalone usage chunk (from stream_options.include_usage).
		if chunk.Usage != nil && len(chunk.Choices) == 0 {
			usage = chunk.Usage
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	// Finalize: send completed event if not already done.
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, usageText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	if !sentCreated {
		if !ensureCreated() {
			return nil, streamErr
		}
	}
	if !sentStop {
		if !sendCompleted(usage) {
			return nil, streamErr
		}
	}

	helper.Done(c)
	return usage, nil
}

// generateResponseID creates a response ID in the format "resp-" + random suffix.
func generateResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	if logID != "" {
		return "resp-" + logID
	}
	return "resp-" + common.GetRandomString(16)
}

// extractChatTextContent extracts text content from a Chat Completions response
// for fallback usage estimation.
func extractChatTextContent(resp *dto.OpenAITextResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	msg := resp.Choices[0].Message
	if msg.IsStringContent() {
		return msg.StringContent()
	}
	parts := msg.ParseContent()
	var sb strings.Builder
	for _, part := range parts {
		if part.Type == dto.ContentTypeText && part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

// buildUsageMap creates a map suitable for inclusion in the response.completed event.
func buildUsageMap(usage *dto.Usage) map[string]any {
	if usage == nil {
		return map[string]any{}
	}
	m := map[string]any{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
		"total_tokens":  usage.TotalTokens,
	}
	if usage.InputTokensDetails != nil {
		m["input_tokens_details"] = map[string]any{
			"cached_tokens": usage.InputTokensDetails.CachedTokens,
		}
	}
	if usage.CompletionTokenDetails.ReasoningTokens != 0 {
		m["output_tokens_details"] = map[string]any{
			"reasoning_tokens": usage.CompletionTokenDetails.ReasoningTokens,
		}
	}
	return m
}
