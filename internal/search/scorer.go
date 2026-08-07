package search

import (
	"strings"

	"github.com/qnqatop/papeer/internal/db"
)

// ScoreResult holds the computed score and human-readable reasons.
type ScoreResult struct {
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

// ScorePaper computes a 0-10 relevance heuristic for a RawPaper
// given the axis keywords. This is a rough pre-filter, not a replacement
// for manual rating.
func ScorePaper(p RawPaper, keywords []db.Keyword) ScoreResult {
	text := strings.ToLower(p.Title + " " + p.Abstract + " " + p.Venue)

	score := 0
	var reasons []string

	// Split keywords into must, boost, exclude.
	var must, boost, exclude []string
	for _, kw := range keywords {
		w := strings.ToLower(kw.Word)
		switch kw.Type {
		case "must":
			must = append(must, w)
		case "boost":
			boost = append(boost, w)
		case "exclude":
			exclude = append(exclude, w)
		}
	}

	// Exclude-keywords: any match → drop the paper immediately.
	if len(exclude) > 0 {
		if hit, kw := firstContains(text, exclude); hit {
			return ScoreResult{0, []string{"excluded by keyword: " + kw}}
		}
	}

	// Must-keywords: if any defined, at least one must match, otherwise score=0.
	if len(must) > 0 {
		if !anyContains(text, must) {
			return ScoreResult{0, []string{"missing must-keywords"}}
		}
		score += 2
		reasons = append(reasons, "must-kw match")
	}

	// Boost-keywords: +1 per hit, capped at 5.
	hits := 0
	for _, kw := range boost {
		if strings.Contains(text, kw) {
			hits++
		}
	}
	if hits > 5 {
		hits = 5
	}
	score += hits
	if hits > 0 {
		reasons = append(reasons, "boost-kw match")
	}

	// +1 for having a meaningful abstract (>100 chars).
	if len(p.Abstract) > 100 {
		score++
		reasons = append(reasons, "has abstract")
	}

	// +1 for good citation count (>20).
	if p.CitationCount > 20 {
		score++
		reasons = append(reasons, "well-cited")
	}

	// +1 for recency (>=2022).
	if p.Year != nil && *p.Year >= 2022 {
		score++
		reasons = append(reasons, "recent")
	}

	// +1 for available open-access PDF.
	if p.PdfURL != "" {
		score++
		reasons = append(reasons, "OA available")
	}

	if score > 10 {
		score = 10
	}
	return ScoreResult{Score: score, Reasons: reasons}
}

func anyContains(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// firstContains returns (true, kw) of the first keyword found in text, or (false, "").
func firstContains(text string, keywords []string) (bool, string) {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true, kw
		}
	}
	return false, ""
}
