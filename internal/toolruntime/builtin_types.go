package toolruntime

type builtinSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type builtinSearchRequest struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
}

type builtinFetchRequest struct {
	URL string `json:"url"`
}

type builtinConfig struct {
	SearchAPI    string
	SearchKey    string
	MaxResults   int
	ProxyURL     string
	MaxFetchSize int64
	FetchTimeout int64
	MaxURLLength int
}

func defaultBuiltinConfig() *builtinConfig {
	return &builtinConfig{
		SearchAPI:    "duckduckgo",
		MaxResults:   10,
		MaxFetchSize: 1 * 1024 * 1024,
		FetchTimeout: 30,
		MaxURLLength: 2000,
	}
}
