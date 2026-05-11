# Responses API Compatibility Report

This document compares the new-api Responses API ↔ Chat Completions conversion implementation
against the OpenAI official spec and litellm's implementation.

## Feature Comparison Matrix

| Feature | OpenAI Spec | litellm | new-api | Notes |
|---------|-------------|---------|---------|-------|
| **Request Conversion** | | | | |
| Simple string input | `"What is 2+2?"` → single user message | Supported | Supported | |
| Array input with conversation items | `[{role, content}, ...]` | Supported | Supported | |
| `input_text` items | → user message | Supported | Supported | |
| `input_image` items | → user message with image_url | Supported | Supported | |
| `input_audio` items | → user message with input_audio | Supported | Supported | |
| `input_file` items | → user message with file | Supported | Supported | |
| `input_video` items | → user message with video_url | N/A | Supported | |
| `function_call` items | → assistant tool_calls | Supported | Supported | |
| `function_call_output` items | → tool message | Supported | Supported | |
| `instructions` → system message | Prepend as system/developer | Supported | Supported | new-api always uses "system" for compatibility |
| `developer` role → `system` | Required for non-o1 models | Supported | Supported | |
| `previous_response_id` | Not convertible | Error | Error | Correct: no Chat Completions equivalent |
| `conversation` | Not convertible | Error | Error | Correct: no Chat Completions equivalent |
| Built-in tools (web_search, file_search) | Not convertible | Skipped | Skipped (warned) | Correct: silently skip for Codex CLI compat |
| `max_output_tokens` → `max_completion_tokens` | Field rename | Supported | Supported | |
| `reasoning.effort` → `reasoning_effort` | Field rename | Supported | Supported | |
| `reasoning.summary` | Responses-only | Not mapped | Not mapped | No Chat Completions equivalent |
| `text.format.type=json_schema` → `response_format` | Nested restructure | Supported | Supported | |
| `text.format.type=json_object` → `response_format` | Type mapping | Supported | Supported | |
| `text.format.type=text` → nil | No format enforcement | Supported | Supported | |
| `tools` flat → nested `function` | `{type,name,parameters}` → `{type,function:{...}}` | Supported | Supported | |
| `tool_choice` flat → nested | `{type,name}` → `{type,function:{name}}` | Supported | Supported | |
| `parameters.type="object"` enforcement | Required by Chat Completions | Enforced | Enforced | Auto-inject when missing |
| `parallel_tool_calls` passthrough | RawMessage → *bool | Supported | Supported | |
| `service_tier` passthrough | string → RawMessage | N/A | Supported | |
| `prompt_cache_key` passthrough | RawMessage → string | N/A | Supported | |
| `logprobs`/`top_logprobs` | Conditional mapping | N/A | Supported | |
| `stream_options` passthrough | Pass through | N/A | Supported | |
| | | | | |
| **Non-Streaming Response Conversion** | | | | |
| `choices[0].message` → `output[]` | Message → output item | Supported | Supported | |
| `reasoning_content` → reasoning output item | Must precede message | Supported | Supported | |
| `reasoning_text` content parts | Fallback for array content | Not handled | Supported | |
| `tool_calls` → `function_call` output items | Per-call items | Supported | Supported | |
| `finish_reason=stop` → `status=completed` | Direct mapping | Supported | Supported | |
| `finish_reason=length` → `status=incomplete` | With `incomplete_details` | Supported | Supported | |
| `finish_reason=content_filter` → `status=incomplete` | With `incomplete_details` | Supported | Supported | |
| `error` → `status=failed` | Direct mapping | Supported | Supported | |
| `usage` mapping | prompt→input, completion→output | Supported | Supported | |
| `instructions` echo-back | Return from original request | Not echoed | Echoed | |
| `metadata` echo-back | Return from original request | Not echoed | Echoed | |
| | | | | |
| **Streaming Response Conversion** | | | | |
| `response.created` event | First event | Supported | Supported | |
| `response.output_item.added` | Before each output item | Supported | Supported | |
| `response.content_part.added` | Before text content | Supported | Supported | |
| `response.output_text.delta` | Text streaming | Supported | Supported | |
| `response.output_text.done` | Text complete | Supported | Supported | |
| `response.content_part.done` | Content part complete | Supported | Supported | |
| `response.output_item.done` | Item complete | Supported | Supported | |
| `response.reasoning_summary_part.added` | Before reasoning text | Not implemented | Supported | |
| `response.reasoning_summary_text.delta` | Reasoning streaming | Not implemented | Supported | |
| `response.reasoning_summary_text.done` | Reasoning complete | Not implemented | Supported | |
| `response.reasoning_summary_part.done` | Reasoning part complete | Not implemented | Supported | |
| `response.function_call_arguments.delta` | Tool call args streaming | Supported | Supported | |
| `response.function_call_arguments.done` | Tool call args complete | Supported | Supported | |
| `response.completed` event | Final event with full output | Supported | Supported | |
| `sequence_number` on all events | Required by OpenAI spec | Not implemented | Supported | |
| Reasoning → message transition | Close reasoning before message | Not implemented | Supported | Explicit close on first non-reasoning delta |
| Reasoning item ID consistency | Same ID in all events | N/A | Consistent | |
| `output_index` tracking | Correct with reasoning offset | Partial | Correct | |
| Usage in completed event | From `stream_options` or estimated | Supported | Supported | |

## Summary of Gaps Addressed

The following gaps were identified by comparing with the OpenAI official spec and litellm,
and have been fixed in this implementation:

1. **`json_object` response format** — `text.format.type=json_object` now correctly maps to `response_format.type=json_object`
2. **`parameters.type="object"` enforcement** — Function tool parameters missing a `type` key now get `type: "object"` auto-injected
3. **`finish_reason` → status mapping** — `length` and `content_filter` now map to `status=incomplete` (not just `incomplete_details`)
4. **`reasoning_text` content parts** — Non-streaming responses handle `reasoning_text` type in array content as fallback
5. **`reasoning.summary` passthrough** — Documented as not applicable (Chat Completions has no equivalent)
6. **Reasoning item ID consistency** — `response.completed` event reuses the same reasoning item ID from SSE events
7. **Reasoning → message transition** — Reasoning item is explicitly closed when first content/tool_call delta arrives mid-stream
8. **`sequence_number`** — All streaming SSE events include a monotonically increasing `sequence_number` per OpenAI spec

## Remaining Known Limitations

These are intentional design choices or features that don't have Chat Completions equivalents:

- **`previous_response_id`** — Not supported (error on conversion attempt)
- **`conversation`** — Not supported (error on conversion attempt)
- **Built-in tools** (web_search_preview, file_search) — Silently skipped with warning
- **`reasoning.summary`** — No Chat Completions equivalent; not forwarded
- **Native image generation** — Not part of this conversion layer
- **Audio output items** — Not yet handled in response conversion

## Web Search Handling Analysis

### litellm's Approach

litellm has a comprehensive web search handling chain:

1. **Request: `web_search_preview` → `web_search_options`** — Converts the Responses API `web_search_preview` tool into the Chat Completions `web_search_options` top-level parameter (with `search_context_size` and `user_location`). OpenAI's Chat Completions API supports this parameter natively.

2. **Response: `annotations` / `url_citation` passthrough** — When Chat Completions returns `annotations` with `url_citation` on message content, litellm converts them to Responses API `response.output_text.annotation.added` stream events and populates `output_text` content part `annotations` arrays.

3. **Input: `web_search_call` items** — When `web_search_call` appears in Responses API input (conversation history), litellm treats it as a tool message (`role=tool`) to preserve search context across turns.

4. **WebSearchInterception** — For providers without native web search (e.g., Bedrock/Claude), litellm intercepts `web_search_preview`, converts it to a `litellm_web_search` function call, executes the search via a built-in search function, and injects results as a tool message.

### Current new-api Gaps

| Capability | litellm | new-api (current) |
|------------|---------|-------------------|
| `web_search_preview` → `web_search_options` | Converted | Dropped silently |
| `url_citation` / annotations passthrough | Full support | DTO field exists but never populated |
| `web_search_call` input items | Converted to tool message | Ignored |
| Web search interception for non-native providers | WebSearchInterception | None |

### Planned Improvements (P1–P4)

- **P1**: Convert `web_search_preview` tool → `web_search_options` parameter for OpenAI-compatible upstreams
- **P2**: Passthrough `url_citation` annotations from Chat Completions responses to Responses API output
- **P3**: Handle `web_search_call` input items as tool messages in multi-turn conversations
- **P4**: WebSearchInterception with configurable search providers (Tavily, Brave, SearXNG) for non-native upstreams

### Search Provider API Reference

| Provider | Endpoint | Auth | Query Field | Result Fields |
|----------|----------|------|-------------|---------------|
| Tavily | `POST https://api.tavily.com/search` | `Authorization: Bearer <key>` | `query` (body) | `results[].title/url/content/score` |
| Brave | `GET https://api.search.brave.com/res/v1/web/search` | `X-Subscription-Token: <key>` | `q` (param) | `web.results[].title/url/description` |
| SearXNG | `GET <base_url>/search?format=json` | None (self-hosted) | `q` (param) | `results[].title/url/content/score` |
