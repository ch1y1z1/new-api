package websearch

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type BraveProvider struct {
	APIKey string
}

type braveResponse struct {
	Web braveWebSection `json:"web"`
}

type braveWebSection struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (b *BraveProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	q := u.Query()
	q.Set("q", query)
	if maxResults > 0 {
		q.Set("count", strconv.Itoa(maxResults))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.APIKey)

	resp, err := defaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &searchError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	var braveResp braveResponse
	if err := common.DecodeJson(resp.Body, &braveResp); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Description,
		})
	}
	return results, nil
}

func (b *BraveProvider) Name() string { return "brave" }
