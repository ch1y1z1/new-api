package openaicompat

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/service/websearch"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ApplyWebSearchInterception checks whether web search interception is needed
// and, if so, executes the search and injects results into the conversation.
// This is called after ResponsesRequestToChatCompletionsRequest when the
// upstream provider does not natively support web search.
func ApplyWebSearchInterception(chatReq *dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) error {
	if chatReq == nil || chatReq.WebSearchOptions == nil {
		return nil
	}

	if !operation_setting.IsWebSearchInterceptionEnabled() {
		return nil
	}

	// Skip interception for upstreams that natively support web_search_options
	// (e.g., OpenAI with openai.com base URL).
	if info != nil && info.ChannelBaseUrl != "" {
		baseUrl := strings.ToLower(info.ChannelBaseUrl)
		if strings.Contains(baseUrl, "openai.com") {
			return nil
		}
	}

	// Extract query from the last user message.
	query := extractLastUserQuery(chatReq.Messages)
	if query == "" {
		return nil
	}

	// Get search provider.
	setting := operation_setting.GetWebSearchSetting()
	provider := websearch.NewProvider(setting.Provider, setting.ApiKey, setting.BaseUrl)
	if provider == nil {
		return fmt.Errorf("unknown web search provider: %s", setting.Provider)
	}

	// Execute search.
	ctx := context.Background()
	maxResults := setting.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	results, err := provider.Search(ctx, query, maxResults)
	if err != nil {
		logger.LogError(nil, "web_search interception failed: "+err.Error())
		return err
	}

	if len(results) == 0 {
		return nil
	}

	// Inject search results as a system context message.
	// We use a system message rather than function_call + tool message pair
	// because some providers (e.g., DeepSeek in thinking mode) reject
	// assistant messages without reasoning_content when tool_calls are present.
	searchContent := formatSearchResults(results)
	searchMsg := dto.Message{
		Role:    "system",
		Content: searchContent,
	}
	// Insert before the last user message so the LLM sees the search
	// results in context before answering.
	lastUserIdx := -1
	for i := len(chatReq.Messages) - 1; i >= 0; i-- {
		if chatReq.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 {
		tail := make([]dto.Message, len(chatReq.Messages[lastUserIdx:]))
		copy(tail, chatReq.Messages[lastUserIdx:])
		chatReq.Messages = append(chatReq.Messages[:lastUserIdx], searchMsg)
		chatReq.Messages = append(chatReq.Messages, tail...)
	} else {
		chatReq.Messages = append(chatReq.Messages, searchMsg)
	}

	// Clear WebSearchOptions since we've handled it.
	chatReq.WebSearchOptions = nil

	// Track usage.
	if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
		if toolInfo, ok := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; ok && toolInfo != nil {
			toolInfo.CallCount++
		}
	}

	return nil
}

func extractLastUserQuery(messages []dto.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			if str, ok := messages[i].Content.(string); ok && str != "" {
				return str
			}
		}
	}
	return ""
}

func formatSearchResults(results []websearch.SearchResult) string {
	var sb strings.Builder
	sb.WriteString("Web search results:\n\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Content))
	}
	return sb.String()
}
