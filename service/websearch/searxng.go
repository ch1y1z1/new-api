package websearch

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type SearXNGProvider struct {
	BaseURL string
}

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func (s *SearXNGProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse(s.BaseURL + "/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	if maxResults > 0 {
		q.Set("pageno", "1")
		_ = strconv.Itoa(maxResults)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &searchError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var sxResp searxngResponse
	if err := common.DecodeJson(resp.Body, &sxResp); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(sxResp.Results))
	for i, r := range sxResp.Results {
		if maxResults > 0 && i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
			Score:   r.Score,
		})
	}
	return results, nil
}

func (s *SearXNGProvider) Name() string { return "searxng" }
