package search

import "github.com/qnqatop/papeer/internal/httpclient"

// ProviderSpec describes one search provider: how to build it and how to
// probe its endpoint for reachability (Settings → check providers).
type ProviderSpec struct {
	Name      string // must equal the provider's Name()
	Lang      string // must equal the provider's Language()
	ProbeURL  string // cheap request that proves the API is reachable
	ProbeJSON bool   // true → probe with DoJSON, false → DoText (XML/HTML)
	New       func(c *httpclient.Client, email string) Provider
}

// Specs returns every search provider in the order the engine runs them.
// It is the single list used by search, radar and the provider check —
// adding a provider here enables it everywhere.
func Specs() []ProviderSpec {
	return []ProviderSpec{
		{
			Name:      "semantic_scholar",
			Lang:      "en",
			ProbeURL:  "https://api.semanticscholar.org/graph/v1/paper/search?query=test&limit=1",
			ProbeJSON: true,
			New:       func(c *httpclient.Client, _ string) Provider { return NewSemanticScholar(c) },
		},
		{
			Name:      "openalex",
			Lang:      "en",
			ProbeURL:  "https://api.openalex.org/works?per-page=1",
			ProbeJSON: true,
			New:       func(c *httpclient.Client, email string) Provider { return NewOpenAlex(c, email) },
		},
		{
			Name:      "crossref",
			Lang:      "en",
			ProbeURL:  "https://api.crossref.org/works?rows=1",
			ProbeJSON: true,
			New:       func(c *httpclient.Client, _ string) Provider { return NewCrossref(c) },
		},
		{
			Name:     "arxiv",
			Lang:     "en",
			ProbeURL: "https://export.arxiv.org/api/query?search_query=test&max_results=1",
			New:      func(c *httpclient.Client, _ string) Provider { return NewArXiv(c) },
		},
		{
			Name:     "cyberleninka",
			Lang:     "ru",
			ProbeURL: "https://cyberleninka.ru",
			New:      func(c *httpclient.Client, email string) Provider { return NewCyberLeninka(c, email) },
		},
	}
}

// NewProviders builds every registered provider on a shared client. email is
// the contact address sent to APIs with a polite pool.
func NewProviders(c *httpclient.Client, email string) []Provider {
	specs := Specs()
	out := make([]Provider, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.New(c, email))
	}
	return out
}
