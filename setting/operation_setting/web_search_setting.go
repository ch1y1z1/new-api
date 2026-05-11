package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type WebSearchSetting struct {
	Provider    string `json:"provider"`     // "tavily", "brave", "searxng", or "" (disabled)
	ApiKey      string `json:"api_key"`      // API key for Tavily/Brave
	BaseUrl     string `json:"base_url"`     // SearXNG base URL (e.g. "http://localhost:8080")
	MaxResults  int    `json:"max_results"`  // max results per search (default 5)
	SearchDepth string `json:"search_depth"` // Tavily: "basic" or "advanced"
}

var webSearchSetting = WebSearchSetting{
	MaxResults:  5,
	SearchDepth: "basic",
}

func init() {
	config.GlobalConfig.Register("web_search_setting", &webSearchSetting)
}

func GetWebSearchSetting() *WebSearchSetting {
	return &webSearchSetting
}

func IsWebSearchInterceptionEnabled() bool {
	return webSearchSetting.Provider != ""
}
