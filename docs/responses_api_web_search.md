# Responses API Web Search Support

This document describes how new-api handles web search when using the Responses API
with providers that do not natively support it (e.g. DeepSeek, Claude via Bedrock).

## Overview

When a client (e.g. Codex CLI) sends a Responses API request with a web search tool:

```json
{
  "model": "deepseek-v4-flash",
  "input": "Latest Go news in 2026?",
  "tools": [{"type": "web_search_preview"}]
}
```

new-api handles this in three layers:

1. **P1 — Web Search Options Passthrough**: Converts `web_search_preview` to the Chat Completions `web_search_options` parameter. For providers that support it natively (e.g. OpenAI), this is all that's needed.

2. **P2 — Annotations Passthrough**: When the upstream returns `url_citation` annotations on message content, they are forwarded to the Responses API output.

3. **P3 — Web Search Call Input Items**: When `web_search_call` items appear in multi-turn conversation history, they are converted to tool messages.

4. **P4 — Web Search Interception**: For providers that **don't** natively support web search (e.g. DeepSeek), new-api intercepts the request, executes the search via a configurable external provider, and injects the results into the conversation.

## How Web Search Interception Works

When the following conditions are all true, interception is triggered:

1. The request contains a `web_search_preview` or `web_search` tool
2. A search provider is configured (see Configuration below)
3. The upstream provider does **not** natively support web search (skipped for `openai.com` base URLs)

The interception flow:

```
Client sends: tools=[{type: "web_search_preview"}], input="What's new in Go?"
                                      |
                                      v
Responses API → Chat Completions conversion
  - web_search_preview → WebSearchOptions on the request
                                      |
                                      v
ApplyWebSearchInterception():
  1. Check if interception is enabled (provider configured)
  2. Check if upstream natively supports web search → skip if yes
  3. Extract search query from last user message
  4. Execute search via configured provider (Tavily/Brave/SearXNG)
  5. Inject search results as a system message before the last user message
  6. Clear WebSearchOptions (already handled)
                                      |
                                      v
Upstream receives: [system msg, ..., "Web search results: ...", user msg]
                  (no web_search_options — provider wouldn't understand it)
```

### Why System Message Injection

Earlier versions used function_call + tool message injection, but some providers
(e.g. DeepSeek in thinking mode) reject assistant messages with `tool_calls` that
don't include `reasoning_content`. The system message approach avoids this constraint
while still giving the model access to search results.

## Reasoning Content Passthrough

When using thinking-mode models (e.g. DeepSeek V4) in multi-turn conversations,
the model requires `reasoning_content` to be passed back on assistant messages from
previous turns. new-api handles this automatically:

1. When a `reasoning` input item precedes an `assistant` message in the conversation
   history, new-api extracts the reasoning text (from `content[].reasoning_text` or
   `summary[].summary_text`) and sets it as `ReasoningContent` on the assistant message.
2. This ensures DeepSeek accepts the multi-turn request without the
   `"The reasoning_content in the thinking mode must be passed back to the API"` error.

## Configuration

### Setting Search Provider

Web search interception is configured via the `web_search_setting` operation setting.
These settings are stored in the `options` table and can be configured through the
admin dashboard (System Settings → Operation Settings) or directly in the database.

| Key | Type | Description |
|-----|------|-------------|
| `web_search_setting.provider` | string | Search provider: `"tavily"`, `"brave"`, `"searxng"`, or `""` (disabled) |
| `web_search_setting.api_key` | string | API key for Tavily or Brave |
| `web_search_setting.base_url` | string | Base URL for SearXNG (e.g. `http://localhost:8080`) |
| `web_search_setting.max_results` | int | Maximum search results per query (default: 5) |
| `web_search_setting.search_depth` | string | Tavily only: `"basic"` or `"advanced"` (default: `"basic"`) |

When `web_search_setting.provider` is empty (default), web search interception is
disabled and `web_search_preview` tools are simply dropped during conversion.

### Via Admin Dashboard (SQL)

```sql
-- Enable Tavily
INSERT INTO options (key, value) VALUES ('web_search_setting', '{"provider":"tavily","api_key":"tvly-xxxxx","max_results":5,"search_depth":"basic"}')
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- Enable Brave
INSERT INTO options (key, value) VALUES ('web_search_setting', '{"provider":"brave","api_key":"BSxxxxx","max_results":5}')
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- Enable SearXNG (self-hosted, no API key needed)
INSERT INTO options (key, value) VALUES ('web_search_setting', '{"provider":"searxng","base_url":"http://localhost:8080","max_results":5}')
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- Disable
INSERT INTO options (key, value) VALUES ('web_search_setting', '{"provider":""}')
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
```

### Via API (cURL)

Settings are synced from the database on a periodic basis. After inserting/updating
the `options` table, the changes will take effect within the next sync cycle (typically
a few seconds), or immediately after restarting the service.

### Search Provider Details

#### Tavily

- **Website**: https://tavily.com
- **API Endpoint**: `POST https://api.tavily.com/search`
- **Auth**: Bearer token in `Authorization` header
- **Free tier**: 1000 requests/month
- **Config**: Set `provider=tavily`, `api_key=tvly-xxxxx`
- **Options**: `search_depth` can be `"basic"` (faster) or `"advanced"` (more thorough)

#### Brave Search

- **Website**: https://brave.com/search/api/
- **API Endpoint**: `GET https://api.search.brave.com/res/v1/web/search`
- **Auth**: `X-Subscription-Token` header
- **Free tier**: 2000 requests/month
- **Config**: Set `provider=brave`, `api_key=BSxxxxx`

#### SearXNG

- **Website**: https://github.com/searxng/searxng
- **API Endpoint**: `GET <base_url>/search?format=json`
- **Auth**: None (self-hosted)
- **Config**: Set `provider=searxng`, `base_url=http://your-searxng:8080`
- **Note**: You must run your own SearXNG instance with JSON format enabled

## Using with Codex CLI

To use new-api as a Responses API backend for Codex CLI with web search support:

1. Configure a search provider (see above)
2. Set up Codex config (`~/.codex/config.toml`):

```toml
model_provider = "custom"
model = "deepseek-v4-flash"

[model_providers.custom]
name = "custom"
wire_api = "responses"
requires_openai_auth = true
base_url = "https://your-new-api.example.com/v1"
```

3. Set the `OPENAI_API_KEY` environment variable to your new-api token

Codex sends `{"type":"web_search","external_web_access":false}` in its tool list.
new-api treats both `web_search` and `web_search_preview` identically — when
interception is enabled, search results will be injected into the conversation
regardless of the `external_web_access` flag.

## Supported Tool Types

| Tool Type | Behavior |
|-----------|----------|
| `web_search_preview` | Converted to `WebSearchOptions`; intercepted if provider configured |
| `web_search` | Same as `web_search_preview` (sent by Codex CLI) |
| `web_search_call` (input item) | Converted to a tool message for multi-turn context |
| `function` | Converted to Chat Completions function tool |
| `file_search` | Silently skipped (no Chat Completions equivalent) |

## Error Handling

Web search interception is **best-effort**: if the search provider fails (invalid API key,
network error, rate limit), the request still proceeds to the upstream model without
search results. The error is logged but does not fail the request.

## Architecture

```
service/openaicompat/
  responses_to_chat_request.go    — P1: web_search_preview → WebSearchOptions
                                  — P3: web_search_call → tool message
                                  — Reasoning content passthrough
  web_search_intercept.go         — P4: ApplyWebSearchInterception()
  chat_to_responses_response.go   — P2: annotations passthrough (non-streaming)

relay/channel/openai/
  chat_to_responses.go            — P2: annotations passthrough (streaming)

service/websearch/
  provider.go                     — SearchProvider interface + factory
  tavily.go                       — Tavily implementation
  brave.go                        — Brave implementation
  searxng.go                      — SearXNG implementation

setting/operation_setting/
  web_search_setting.go           — Configuration struct + registration
```
