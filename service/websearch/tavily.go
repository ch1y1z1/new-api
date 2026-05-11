package websearch

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type TavilyProvider struct {
	APIKey string
}

type tavilyRequest struct {
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func (t *TavilyProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	body := tavilyRequest{
		Query:         query,
		SearchDepth:   "basic",
		MaxResults:    maxResults,
		IncludeAnswer: false,
	}
	bodyBytes, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.APIKey)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &searchError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var tavResp tavilyResponse
	if err := common.DecodeJson(resp.Body, &tavResp); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(tavResp.Results))
	for _, r := range tavResp.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
			Score:   r.Score,
		})
	}
	return results, nil
}

func (t *TavilyProvider) Name() string { return "tavily" }
