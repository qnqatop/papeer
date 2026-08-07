package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

// CitationFetcher fetches citation data from Semantic Scholar paper detail API.
type CitationFetcher struct {
	client *httpclient.Client
}

// NewCitationFetcher creates a new CitationFetcher.
func NewCitationFetcher(c *httpclient.Client) *CitationFetcher {
	return &CitationFetcher{client: c}
}

// S2CitationEntry represents a paper returned by the S2 references/citations endpoint.
type S2CitationEntry struct {
	S2PaperID     string     `json:"paperId"`
	Title         string     `json:"title"`
	Year          *int       `json:"year"`
	Authors       []s2Author `json:"authors"`
	ExternalIDs   s2ExtIDs   `json:"externalIds"`
	CitationCount *int       `json:"citationCount"`
}

// NewS2CitationEntryWithDOI constructs an entry with a given DOI in the
// external IDs. Test helper for callers outside the search package — keeps
// s2ExtIDs unexported while still allowing tests to seed DOI matches.
func NewS2CitationEntryWithDOI(title, doi string) S2CitationEntry {
	return S2CitationEntry{
		Title:       title,
		ExternalIDs: s2ExtIDs{DOI: doi},
	}
}

// ResolvePaperID determines the S2 lookup key for a paper (DOI: or ARXIV: prefix).
func ResolvePaperID(doi *string, arxivID *string, s2ID *string) string {
	if s2ID != nil && *s2ID != "" {
		return *s2ID
	}
	if doi != nil && *doi != "" {
		return "DOI:" + *doi
	}
	if arxivID != nil && *arxivID != "" {
		return "ARXIV:" + *arxivID
	}
	return ""
}

// FetchReferences fetches papers that a given paper references (its bibliography).
func (f *CitationFetcher) FetchReferences(ctx context.Context, paperID string, limit int) ([]S2CitationEntry, error) {
	return f.fetchCitations(ctx, paperID, "references", limit)
}

// FetchCitedBy fetches papers that cite the given paper.
func (f *CitationFetcher) FetchCitedBy(ctx context.Context, paperID string, limit int) ([]S2CitationEntry, error) {
	return f.fetchCitations(ctx, paperID, "citations", limit)
}

func (f *CitationFetcher) fetchCitations(ctx context.Context, paperID, direction string, limit int) ([]S2CitationEntry, error) {
	fields := "title,year,authors,externalIds,citationCount"
	u := fmt.Sprintf(
		"https://api.semanticscholar.org/graph/v1/paper/%s/%s?fields=%s&limit=%d",
		url.PathEscape(paperID), direction, fields, limit,
	)

	var resp s2CitationResponse
	if err := f.client.DoJSONWithRetry(ctx, u, &resp, 2); err != nil {
		return nil, err
	}

	entries := make([]S2CitationEntry, 0, len(resp.Data))
	for _, item := range resp.Data {
		var entry S2CitationEntry
		if direction == "references" {
			entry = item.CitedPaper
		} else {
			entry = item.CitingPaper
		}
		if entry.S2PaperID == "" || entry.Title == "" {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ExtractAuthors extracts author names from S2CitationEntry.
func ExtractAuthors(authors []s2Author) []string {
	out := make([]string, 0, len(authors))
	for _, a := range authors {
		if name := strings.TrimSpace(a.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// CitationCountVal safely extracts citation count.
func CitationCountVal(entry S2CitationEntry) int {
	return ptr.Val(entry.CitationCount)
}

// ResolveS2PaperID resolves a DOI/ArXiv ID to a Semantic Scholar paper ID.
func (f *CitationFetcher) ResolveS2PaperID(ctx context.Context, lookupKey string) (string, error) {
	u := fmt.Sprintf("https://api.semanticscholar.org/graph/v1/paper/%s?fields=paperId",
		url.PathEscape(lookupKey))

	var resp struct {
		PaperID string `json:"paperId"`
	}
	if err := f.client.DoJSONWithRetry(ctx, u, &resp, 1); err != nil {
		return "", err
	}
	return resp.PaperID, nil
}

type s2CitationResponse struct {
	Data []s2CitationItem `json:"data"`
}

type s2CitationItem struct {
	CitedPaper  S2CitationEntry `json:"citedPaper"`
	CitingPaper S2CitationEntry `json:"citingPaper"`
}

// FetchPaperDetail fetches full metadata (title, abstract, year, venue,
// authors, external IDs, open-access PDF, citation count) for a single S2
// paper ID. Used when promoting a Coverage Gaps external citation to a full
// paper (ResolveAndAddExternal) — external_citations only stores the
// minimal fields collected while walking references/citations.
func (f *CitationFetcher) FetchPaperDetail(ctx context.Context, s2ID string) (RawPaper, error) {
	fields := "title,abstract,year,venue,authors,externalIds,openAccessPdf,citationCount"
	u := fmt.Sprintf("https://api.semanticscholar.org/graph/v1/paper/%s?fields=%s",
		url.PathEscape(s2ID), fields)

	var resp s2Paper
	if err := f.client.DoJSONWithRetry(ctx, u, &resp, 2); err != nil {
		return RawPaper{}, err
	}

	doi := strings.ToLower(resp.ExternalIDs.DOI)
	var pdfURL string
	if resp.OpenAccessPdf != nil && resp.OpenAccessPdf.Status != "CLOSED" {
		pdfURL = resp.OpenAccessPdf.URL
	}

	return RawPaper{
		Title:         strings.TrimSpace(resp.Title),
		Abstract:      strings.TrimSpace(resp.Abstract),
		Year:          resp.Year,
		Venue:         strings.TrimSpace(resp.Venue),
		Authors:       extractS2Authors(resp.Authors),
		DOI:           doi,
		ArxivID:       resp.ExternalIDs.ArXiv,
		PdfURL:        pdfURL,
		CitationCount: ptr.Val(resp.CitationCount),
		Source:        "semantic_scholar",
	}, nil
}

// S2BatchPaper is one entry of a Semantic Scholar /paper/batch response: the
// resolved paper's own identity plus its embedded references/citations.
type S2BatchPaper struct {
	S2PaperID     string            `json:"paperId"`
	Title         string            `json:"title"`
	Year          *int              `json:"year"`
	Authors       []s2Author        `json:"authors"`
	ExternalIDs   s2ExtIDs          `json:"externalIds"`
	CitationCount *int              `json:"citationCount"`
	References    []S2CitationEntry `json:"references"`
	Citations     []S2CitationEntry `json:"citations"`
}

// BatchSize is the max number of paper IDs Semantic Scholar's /paper/batch
// endpoint accepts per request (S2-documented limit).
const BatchSize = 500

// batchFields asks the batch endpoint for each paper's own identity plus its
// embedded references and citations.
//
// IMPORTANT LIMITATION: unlike the dedicated /paper/{id}/references and
// /paper/{id}/citations endpoints (which paginate via offset/limit up to
// their own caps), the batch endpoint returns references/citations inline
// with no pagination, and Semantic Scholar truncates each list at a few
// hundred to ~1000 entries per paper (undocumented exact cap, observed
// empirically). For citation-graph purposes (finding internal links and
// building the Coverage Gaps candidate list) this is more than sufficient —
// it is not a guarantee of a complete bibliography for extremely
// highly-cited papers.
const batchFields = "paperId,title,year,authors,externalIds,citationCount," +
	"references.paperId,references.title,references.year,references.authors,references.externalIds,references.citationCount," +
	"citations.paperId,citations.title,citations.year,citations.authors,citations.externalIds,citations.citationCount"

// FetchBatch resolves a batch of lookup keys (bare S2 paper ids, "DOI:...",
// "ARXIV:..." — see ResolvePaperID) to their full S2 identity plus
// references and citations, in as few HTTP requests as possible instead of
// one (or three) requests per paper. This is the fast path used by the
// citation-fetch worker: it turns what used to be ~3 sequential requests per
// paper (resolve + references + citedBy) at ~0.33-1 req/s into one request
// per up-to-500 papers.
//
// The returned slice has the same length and order as ids; an entry is nil
// wherever Semantic Scholar could not resolve that id (returns null in that
// position, e.g. an unknown DOI).
func (f *CitationFetcher) FetchBatch(ctx context.Context, ids []string) ([]*S2BatchPaper, error) {
	out := make([]*S2BatchPaper, 0, len(ids))
	for start := 0; start < len(ids); start += BatchSize {
		end := start + BatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk, err := f.fetchBatchChunk(ctx, ids[start:end])
		if err != nil {
			return out, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func (f *CitationFetcher) fetchBatchChunk(ctx context.Context, ids []string) ([]*S2BatchPaper, error) {
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: ids})
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("https://api.semanticscholar.org/graph/v1/paper/batch?fields=%s", url.QueryEscape(batchFields))

	var lastErr error
	backoff := []time.Duration{3 * time.Second, 8 * time.Second}
	for attempt := 0; attempt <= len(backoff); attempt++ {
		respBody, status, err := f.client.PostJSON(ctx, u, body, nil)
		if err != nil {
			lastErr = err
		} else if status >= 400 {
			lastErr = &httpclient.StatusError{Code: status, Host: "api.semanticscholar.org"}
		} else {
			var parsed []*S2BatchPaper
			if uerr := json.Unmarshal(respBody, &parsed); uerr != nil {
				return nil, fmt.Errorf("decode batch response: %w", uerr)
			}
			return parsed, nil
		}

		retryable := status == 429 || status >= 500
		if attempt < len(backoff) && retryable {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff[attempt]):
			}
			continue
		}
		break
	}
	return nil, lastErr
}
