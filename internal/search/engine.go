package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/ptr"
)

// httpStatusCode extracts the HTTP status from a *httpclient.StatusError, or 0.
func httpStatusCode(err error) int {
	var se *httpclient.StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return 0
}

// SearchEvent is emitted during search to report progress.
type SearchEvent struct {
	Type       string `json:"type"`        // "query_start", "provider_done", "provider_error", "query_done", "axis_done", "done"
	Axis       string `json:"axis"`        // axis name
	Query      string `json:"query"`       // current query
	Provider   string `json:"provider"`    // provider name
	Count      int    `json:"count"`       // papers found (for provider_done)
	Total      int    `json:"total"`       // total deduped (for axis_done/done)
	Error      string `json:"error"`       // error message (for provider_error)
	DurationMs int64  `json:"duration_ms"` // provider/query duration in ms
	RawCount   int    `json:"raw_count"`   // before dedup (for query_done/axis_done)
}

// EventFunc receives search progress events.
type EventFunc func(SearchEvent)

// Engine orchestrates parallel search across providers, deduplication, scoring,
// and upserting results into the database.
type Engine struct {
	providers     []Provider
	database      *db.DB
	onEvent       EventFunc
	skipProviders map[string]bool // circuit breaker persisted across SearchAxis calls
}

// NewEngine creates a search engine with given providers.
func NewEngine(database *db.DB, providers []Provider, onEvent EventFunc) *Engine {
	if onEvent == nil {
		onEvent = func(SearchEvent) {}
	}
	return &Engine{
		providers:     providers,
		database:      database,
		onEvent:       onEvent,
		skipProviders: make(map[string]bool),
	}
}

// SearchAxisInput contains parameters for searching one axis.
type SearchAxisInput struct {
	ProfileID   int64
	Axis        db.Axis
	Queries     []string
	Keywords    []db.Keyword
	YearMin     int
	MaxPerQuery int
}

// SearchAxis runs all queries for a single axis across all providers, deduplicates,
// scores, and upserts results. Returns the number of new/updated papers.
func (e *Engine) SearchAxis(ctx context.Context, input SearchAxisInput) (int, error) {
	var allRaw []RawPaper

	for _, query := range input.Queries {
		e.onEvent(SearchEvent{
			Type:  "query_start",
			Axis:  input.Axis.AxisKey,
			Query: query,
		})

		qStart := time.Now()
		results, newSkips := e.searchQueryParallel(ctx, query, input.MaxPerQuery, input.YearMin, input.Axis.AxisKey, e.skipProviders)
		for name, reason := range newSkips {
			if !e.skipProviders[name] {
				e.skipProviders[name] = true
				e.onEvent(SearchEvent{
					Type:     "provider_error",
					Axis:     input.Axis.AxisKey,
					Provider: name,
					Error:    "circuit breaker: " + reason,
				})
			}
		}
		allRaw = append(allRaw, results...)

		e.onEvent(SearchEvent{
			Type:       "query_done",
			Axis:       input.Axis.AxisKey,
			Query:      query,
			Count:      len(results),
			DurationMs: time.Since(qStart).Milliseconds(),
		})
	}

	// Deduplicate across all queries and providers.
	rawTotal := len(allRaw)
	deduped := Dedupe(allRaw)

	// Score and upsert.
	upserted := 0
	for _, raw := range deduped {
		sr := ScorePaper(raw, input.Keywords)

		sources := strings.Split(raw.Source, ",")

		paper := &db.Paper{
			ProfileID:       input.ProfileID,
			AxisID:          &input.Axis.ID,
			Title:           raw.Title,
			TitleNormalized: NormalizeTitle(raw.Title),
			Abstract:        raw.Abstract,
			Year:            raw.Year,
			Venue:           raw.Venue,
			Authors:         sources2authors(raw.Authors),
			DOI:             ptr.Ptr(raw.DOI),
			ArxivID:         ptr.Ptr(raw.ArxivID),
			PdfURL:          ptr.Ptr(raw.PdfURL),
			PdfSource:       ptr.Ptr(raw.Source),
			CitationCount:   raw.CitationCount,
			PreScore:        sr.Score,
			ScoreReasons:    sr.Reasons,
			Sources:         sources,
			Status:          "new",
		}

		if err := e.database.UpsertPaper(paper); err != nil {
			return upserted, fmt.Errorf("upsert paper %q: %w", raw.Title, err)
		}
		upserted++
	}

	e.onEvent(SearchEvent{
		Type:     "axis_done",
		Axis:     input.Axis.AxisKey,
		Total:    upserted,
		RawCount: rawTotal,
	})

	return upserted, nil
}

// searchQueryParallel runs all providers in parallel for a single query.
// skipProviders lists providers to skip (circuit breaker).
// Returns results and a map of newly tripped providers → human-readable reason.
func (e *Engine) searchQueryParallel(ctx context.Context, query string, limit, yearMin int, axisName string, skipProviders map[string]bool) ([]RawPaper, map[string]string) {
	type result struct {
		papers     []RawPaper
		err        error
		name       string
		durationMs int64
	}

	// Filter out skipped providers.
	var active []Provider
	for _, p := range e.providers {
		if !skipProviders[p.Name()] {
			active = append(active, p)
		}
	}

	ch := make(chan result, len(active))
	var wg sync.WaitGroup

	for _, p := range active {
		wg.Add(1)
		go func(prov Provider) {
			defer wg.Done()
			start := time.Now()
			papers, err := prov.Search(ctx, query, limit, yearMin)
			ch <- result{papers: papers, err: err, name: prov.Name(), durationMs: time.Since(start).Milliseconds()}
		}(p)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []RawPaper
	newSkips := make(map[string]string)
	for r := range ch {
		if r.err != nil {
			e.onEvent(SearchEvent{
				Type:       "provider_error",
				Axis:       axisName,
				Query:      query,
				Provider:   r.name,
				Error:      r.err.Error(),
				DurationMs: r.durationMs,
			})
			// Trip circuit breaker on rate limit (429) or geo/auth block (403).
			// 403 catches Semantic Scholar denying RU IPs without VPN.
			if reason := providerUnavailableReason(r.err); reason != "" {
				newSkips[r.name] = reason
			}
			continue
		}
		e.onEvent(SearchEvent{
			Type:       "provider_done",
			Axis:       axisName,
			Query:      query,
			Provider:   r.name,
			Count:      len(r.papers),
			DurationMs: r.durationMs,
		})
		all = append(all, r.papers...)
	}

	return all, newSkips
}

// providerUnavailableReason returns a short human reason string if the error
// should trip the circuit breaker, or "" if not. Distinguishes 403 (geo/auth
// block — likely RU IP without VPN, or missing Semantic Scholar API key) from
// 429 (rate limited).
func providerUnavailableReason(err error) string {
	if err == nil {
		return ""
	}
	switch httpStatusCode(err) {
	case 403:
		return "skipping for remaining queries (HTTP 403 — provider blocked; check VPN or add API key in Settings)"
	case 429:
		return "skipping for remaining queries (rate limited)"
	}
	return ""
}

// ProviderBlockReason returns a short, UI-friendly reason if err looks like a
// provider geo/auth block (403) or rate limit (429). Empty string otherwise.
// Used by callers outside the search engine (e.g. citation fetcher) that need
// to surface the same kind of "provider unavailable" message to the UI.
func ProviderBlockReason(err error) string {
	if err == nil {
		return ""
	}
	switch httpStatusCode(err) {
	case 403:
		return "HTTP 403 — Semantic Scholar blocked this request. Check VPN or add an API key in Settings."
	case 429:
		return "Rate limited by Semantic Scholar. Add an API key in Settings to raise the quota."
	}
	return ""
}

func sources2authors(authors []string) db.JSONStringSlice {
	if len(authors) == 0 {
		return nil
	}
	return db.JSONStringSlice(authors)
}
