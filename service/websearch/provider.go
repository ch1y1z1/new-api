package websearch

import (
	"context"
	"net/http"
	"time"
)

type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

type SearchProvider interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
	Name() string
}

func NewProvider(providerType, apiKey, baseUrl string) SearchProvider {
	switch providerType {
	case "tavily":
		return &TavilyProvider{APIKey: apiKey}
	case "brave":
		return &BraveProvider{APIKey: apiKey}
	case "searxng":
		return &SearXNGProvider{BaseURL: baseUrl}
	default:
		return nil
	}
}

var defaultClient = &http.Client{Timeout: 15 * time.Second}

type searchError struct {
	StatusCode int
	Message    string
}

func (e *searchError) Error() string {
	return e.Message
}
