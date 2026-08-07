package search

import (
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
)

func TestScorePaper_MustKeywordMissing(t *testing.T) {
	p := RawPaper{Title: "Deep Learning Survey", Abstract: "About neural networks"}
	kw := []db.Keyword{{Word: "cold start", Type: "must"}}

	r := ScorePaper(p, kw)
	if r.Score != 0 {
		t.Errorf("score = %d, want 0", r.Score)
	}
}

func TestScorePaper_MustKeywordPresent(t *testing.T) {
	p := RawPaper{Title: "Cold Start in RecSys", Abstract: "About cold start problems"}
	kw := []db.Keyword{{Word: "cold start", Type: "must"}}

	r := ScorePaper(p, kw)
	if r.Score < 2 {
		t.Errorf("score = %d, want >= 2", r.Score)
	}
}

func TestScorePaper_BoostKeywords(t *testing.T) {
	p := RawPaper{
		Title:    "Collaborative Filtering with Deep Learning",
		Abstract: "We use embeddings and matrix factorization for recommendations",
	}
	kw := []db.Keyword{
		{Word: "collaborative filtering", Type: "boost"},
		{Word: "deep learning", Type: "boost"},
		{Word: "embeddings", Type: "boost"},
		{Word: "unrelated", Type: "boost"},
	}

	r := ScorePaper(p, kw)
	// 3 boost hits
	if r.Score != 3 {
		t.Errorf("score = %d, want 3", r.Score)
	}
}

func TestScorePaper_BoostCappedAt5(t *testing.T) {
	p := RawPaper{
		Title:    "a b c d e f g",
		Abstract: "",
	}
	kw := []db.Keyword{
		{Word: "a", Type: "boost"}, {Word: "b", Type: "boost"},
		{Word: "c", Type: "boost"}, {Word: "d", Type: "boost"},
		{Word: "e", Type: "boost"}, {Word: "f", Type: "boost"},
		{Word: "g", Type: "boost"},
	}

	r := ScorePaper(p, kw)
	// 7 hits but capped at 5
	if r.Score != 5 {
		t.Errorf("score = %d, want 5 (capped)", r.Score)
	}
}

func TestScorePaper_AllBonuses(t *testing.T) {
	y := 2023
	p := RawPaper{
		Title:         "Cold Start Recommender",
		Abstract:      "This is a long abstract about cold start problems in recommender systems that spans over one hundred characters easily for testing purposes and more text here.",
		Year:          &y,
		CitationCount: 50,
		PdfURL:        "https://arxiv.org/pdf/123.pdf",
	}
	kw := []db.Keyword{
		{Word: "cold start", Type: "must"},
		{Word: "recommender", Type: "boost"},
	}

	r := ScorePaper(p, kw)
	// must=2 + boost=1 + abstract=1 + cited=1 + recent=1 + OA=1 = 7
	if r.Score != 7 {
		t.Errorf("score = %d, want 7", r.Score)
	}
}

func TestScorePaper_ExcludeKeywordHit(t *testing.T) {
	// Must-keyword "contamination" matches, but exclude-keyword "language model" also matches → drop.
	p := RawPaper{
		Title:    "A Taxonomy for Data Contamination in Large Language Models",
		Abstract: "We survey contamination in LLM pretraining datasets.",
	}
	kw := []db.Keyword{
		{Word: "contamination", Type: "must"},
		{Word: "language model", Type: "exclude"},
	}
	r := ScorePaper(p, kw)
	if r.Score != 0 {
		t.Errorf("score = %d, want 0 (excluded)", r.Score)
	}
	if len(r.Reasons) == 0 || !strings.HasPrefix(r.Reasons[0], "excluded by keyword") {
		t.Errorf("reasons = %v, want exclusion reason first", r.Reasons)
	}
}

func TestScorePaper_ExcludeKeywordMiss(t *testing.T) {
	// Exclude defined but not matching → normal scoring proceeds.
	y := 2023
	p := RawPaper{
		Title:         "Heavy Metal Contamination in River Sediments",
		Abstract:      "This is a long abstract about lead, zinc and copper contamination in sediments of a polluted river system, including measurements from multiple sites.",
		Year:          &y,
		CitationCount: 50,
		PdfURL:        "https://example.com/p.pdf",
	}
	kw := []db.Keyword{
		{Word: "contamination", Type: "must"},
		{Word: "sediments", Type: "boost"},
		{Word: "language model", Type: "exclude"},
	}
	r := ScorePaper(p, kw)
	// must=2 + boost=1 + abstract=1 + cited=1 + recent=1 + OA=1 = 7
	if r.Score != 7 {
		t.Errorf("score = %d, want 7 (exclude should not fire)", r.Score)
	}
}

func TestScorePaper_NoKeywords(t *testing.T) {
	y := 2020
	p := RawPaper{
		Title:         "Some Paper",
		Abstract:      "Short",
		Year:          &y,
		CitationCount: 5,
	}

	r := ScorePaper(p, nil)
	// No must/boost, short abstract, low citations, old, no PDF → 0
	if r.Score != 0 {
		t.Errorf("score = %d, want 0", r.Score)
	}
}

func TestScorePaper_MaxCap(t *testing.T) {
	y := 2024
	p := RawPaper{
		Title:         "a b c d e cold start",
		Abstract:      "This is a very long abstract to pass the 100 char check. We discuss a b c d e cold start and many other topics in recommender systems and beyond.",
		Year:          &y,
		CitationCount: 100,
		PdfURL:        "https://example.com/paper.pdf",
	}
	kw := []db.Keyword{
		{Word: "cold start", Type: "must"},
		{Word: "a", Type: "boost"}, {Word: "b", Type: "boost"},
		{Word: "c", Type: "boost"}, {Word: "d", Type: "boost"},
		{Word: "e", Type: "boost"}, {Word: "recommender", Type: "boost"},
	}

	r := ScorePaper(p, kw)
	// must=2 + boost=5(capped) + abstract=1 + cited=1 + recent=1 + OA=1 = 11 → capped to 10
	if r.Score != 10 {
		t.Errorf("score = %d, want 10 (capped)", r.Score)
	}
}
