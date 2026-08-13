package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

// mockProvider returns pre-defined papers.
type mockProvider struct {
	name   string
	lang   string
	papers []RawPaper
	err    error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Language() string {
	if m.lang == "" {
		return "en"
	}
	return m.lang
}
func (m *mockProvider) Search(_ context.Context, _ string, _ int, _ int) ([]RawPaper, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.papers, nil
}

func testSearchDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func createTestSearchProfile(t *testing.T, d *db.DB) int64 {
	t.Helper()
	p := &db.Profile{Name: "test", Email: "test@test.com"}
	if err := d.CreateProfile(p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestEngine_SearchAxis_MockProviders(t *testing.T) {
	d := testSearchDB(t)
	profileID := createTestSearchProfile(t, d)

	// Create an axis.
	axis := &db.Axis{
		ProfileID: profileID,
		AxisKey:   "cold_start",
		Queries:   []db.Query{{Text: "cold start recommender"}},
		Keywords: []db.Keyword{
			{Word: "cold start", Type: "must"},
			{Word: "recommender", Type: "boost"},
		},
	}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	y2023 := 2023
	y2021 := 2021

	providers := []Provider{
		&mockProvider{
			name: "semantic_scholar",
			papers: []RawPaper{
				{
					Title:         "Cold Start in Recommender Systems",
					Abstract:      "A comprehensive study of cold start problems in recommender systems with deep learning approaches applied to user modeling.",
					Year:          &y2023,
					DOI:           "10.1234/cs",
					CitationCount: 50,
					PdfURL:        "https://s2.com/paper.pdf",
					Authors:       []string{"John Doe"},
					Source:        "semantic_scholar",
				},
			},
		},
		&mockProvider{
			name: "openalex",
			papers: []RawPaper{
				{
					Title:         "Cold Start in Recommender Systems", // same paper, will dedupe by title
					Abstract:      "A study of cold start",
					Year:          &y2023,
					DOI:           "10.1234/cs",
					CitationCount: 45,
					Authors:       []string{"John Doe"},
					Source:        "openalex",
				},
				{
					Title:         "Another Cold Start Paper",
					Abstract:      "Different paper about cold start in recommendations",
					Year:          &y2021,
					CitationCount: 10,
					Authors:       []string{"Jane Smith"},
					Source:        "openalex",
				},
			},
		},
	}

	var events []SearchEvent
	var mu sync.Mutex
	onEvent := func(e SearchEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	engine := NewEngine(d, providers, onEvent)

	count, err := engine.SearchAxis(context.Background(), SearchAxisInput{
		ProfileID:   profileID,
		Axis:        *axis,
		Queries:     []string{"cold start recommender"},
		Keywords:    axis.Keywords,
		YearMin:     2018,
		MaxPerQuery: 25,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should dedupe to 2 unique papers.
	if count != 2 {
		t.Errorf("upserted = %d, want 2", count)
	}

	// Verify papers in DB.
	papers, total, err := d.ListPapers(db.PaperFilter{ProfileID: profileID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Errorf("total in DB = %d, want 2", total)
	}

	// First paper should have merged sources.
	for _, p := range papers {
		if p.DOI != nil && *p.DOI == "10.1234/cs" {
			if len(p.Sources) < 2 {
				t.Errorf("sources = %v, want 2 sources", p.Sources)
			}
			if p.CitationCount != 50 {
				t.Errorf("citations = %d, want 50 (max)", p.CitationCount)
			}
		}
	}

	// Check events were emitted.
	mu.Lock()
	defer mu.Unlock()
	hasQueryStart := false
	hasAxisDone := false
	for _, e := range events {
		if e.Type == "query_start" {
			hasQueryStart = true
		}
		if e.Type == "axis_done" {
			hasAxisDone = true
		}
	}
	if !hasQueryStart {
		t.Error("missing query_start event")
	}
	if !hasAxisDone {
		t.Error("missing axis_done event")
	}
}

func TestEngine_ProviderError(t *testing.T) {
	d := testSearchDB(t)
	profileID := createTestSearchProfile(t, d)

	axis := &db.Axis{
		ProfileID: profileID,
		AxisKey:   "test",
		Queries:   []db.Query{{Text: "test query"}},
	}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	providers := []Provider{
		&mockProvider{name: "failing", err: fmt.Errorf("API down")},
		&mockProvider{
			name: "working",
			papers: []RawPaper{
				{Title: "Good Paper", Abstract: "Content", Source: "working"},
			},
		},
	}

	var errorEvents []SearchEvent
	var mu sync.Mutex
	engine := NewEngine(d, providers, func(e SearchEvent) {
		mu.Lock()
		if e.Type == "provider_error" {
			errorEvents = append(errorEvents, e)
		}
		mu.Unlock()
	})

	count, err := engine.SearchAxis(context.Background(), SearchAxisInput{
		ProfileID:   profileID,
		Axis:        *axis,
		Queries:     []string{"test query"},
		YearMin:     2018,
		MaxPerQuery: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should still upsert the paper from the working provider.
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(errorEvents) != 1 {
		t.Errorf("error events = %d, want 1", len(errorEvents))
	}
}

// langScopeProviders returns one English and one Russian mock provider, each
// tagging its single paper's Source so we can tell which one ran.
func langScopeProviders() []Provider {
	return []Provider{
		&mockProvider{
			name: "semantic_scholar",
			lang: "en",
			papers: []RawPaper{
				{Title: "English Recommender Paper", Abstract: "English content about recommenders", Source: "semantic_scholar"},
			},
		},
		&mockProvider{
			name: "cyberleninka",
			lang: "ru",
			papers: []RawPaper{
				{Title: "Рекомендательная система для абитуриентов", Abstract: "Русскоязычная статья про рекомендательные системы", Source: "cyberleninka"},
			},
		},
	}
}

func runScopedAxis(t *testing.T, scope string) []db.Paper {
	t.Helper()
	d := testSearchDB(t)
	profileID := createTestSearchProfile(t, d)

	axis := &db.Axis{
		ProfileID: profileID,
		AxisKey:   "scoped",
		Queries:   []db.Query{{Text: "recommender"}},
		LangScope: scope,
	}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	// SaveAxis normalizes an empty scope to "en"; feed the raw scope to the
	// engine so the empty-scope default is exercised at the engine level.
	axisInput := *axis
	axisInput.LangScope = scope

	engine := NewEngine(d, langScopeProviders(), nil)
	if _, err := engine.SearchAxis(context.Background(), SearchAxisInput{
		ProfileID:   profileID,
		Axis:        axisInput,
		Queries:     []string{"recommender"},
		MaxPerQuery: 25,
	}); err != nil {
		t.Fatal(err)
	}

	papers, _, err := d.ListPapers(db.PaperFilter{ProfileID: profileID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return papers
}

func TestEngine_LangScope_RuUsesOnlyRussianProviders(t *testing.T) {
	papers := runScopedAxis(t, "ru")
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d, want 1", len(papers))
	}
	if papers[0].PdfSource == nil || *papers[0].PdfSource != "cyberleninka" {
		t.Errorf("source = %v, want cyberleninka only", papers[0].PdfSource)
	}
}

func TestEngine_LangScope_EnUsesOnlyEnglishProviders(t *testing.T) {
	papers := runScopedAxis(t, "en")
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d, want 1", len(papers))
	}
	if papers[0].PdfSource == nil || *papers[0].PdfSource != "semantic_scholar" {
		t.Errorf("source = %v, want semantic_scholar only", papers[0].PdfSource)
	}
}

func TestEngine_LangScope_EmptyDefaultsToEnglish(t *testing.T) {
	// A legacy axis with no stored lang_scope must behave as English-only.
	papers := runScopedAxis(t, "")
	if len(papers) != 1 {
		t.Fatalf("len(papers) = %d, want 1", len(papers))
	}
	if papers[0].PdfSource == nil || *papers[0].PdfSource != "semantic_scholar" {
		t.Errorf("source = %v, want semantic_scholar (English default)", papers[0].PdfSource)
	}
}

func TestEngine_LangScope_FiltersOffLanguageArticles(t *testing.T) {
	d := testSearchDB(t)
	profileID := createTestSearchProfile(t, d)

	axis := &db.Axis{
		ProfileID: profileID,
		AxisKey:   "mixed",
		Queries:   []db.Query{{Text: "рекомендации"}},
		LangScope: "ru",
	}
	if err := d.SaveAxis(axis); err != nil {
		t.Fatal(err)
	}

	// A ru-scoped source returns a mixed batch: two Russian articles and one
	// English one that slipped through. Only the Russian ones must survive.
	providers := []Provider{
		&mockProvider{
			name: "cyberleninka",
			lang: "ru",
			papers: []RawPaper{
				{Title: "Рекомендательная система для абитуриентов", Abstract: "Про рекомендации", Source: "cyberleninka"},
				{Title: "Deep Learning for Recommendations", Abstract: "An English article", Source: "cyberleninka"},
				{Title: "Гибридные методы рекомендаций", Abstract: "Ещё одна русская статья", Source: "cyberleninka"},
			},
		},
	}

	engine := NewEngine(d, providers, nil)
	if _, err := engine.SearchAxis(context.Background(), SearchAxisInput{
		ProfileID:   profileID,
		Axis:        *axis,
		Queries:     []string{"рекомендации"},
		MaxPerQuery: 25,
	}); err != nil {
		t.Fatal(err)
	}

	papers, total, err := d.ListPapers(db.PaperFilter{ProfileID: profileID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (English article filtered out)", total)
	}
	for _, p := range papers {
		if p.Title == "Deep Learning for Recommendations" {
			t.Error("English article should have been filtered out of a ru-scoped axis")
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
