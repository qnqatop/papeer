package app

import (
	"fmt"
	"sort"
)

// GenerateYearDistributionParagraph builds a short, deterministic
// human-readable paragraph describing the profile's year distribution
// (range, median, rough trend) — backs the "insert paragraph into report"
// button next to the Year Distribution chart. No LLM involved, purely
// descriptive statistics, so it always works even without an LLM profile
// configured.
func (a *App) GenerateYearDistributionParagraph(profileID int64, axisID *int64) (string, error) {
	yearDist, err := a.db.YearDistribution(profileID, axisID)
	if err != nil {
		return "", err
	}
	if len(yearDist) == 0 {
		return "", fmt.Errorf("no dated papers to summarize")
	}

	years := make([]int, 0, len(yearDist))
	total := 0
	for y, c := range yearDist {
		years = append(years, y)
		total += c
	}
	sort.Ints(years)
	minY, maxY := years[0], years[len(years)-1]

	// Paper-weighted median publication year.
	expanded := make([]int, 0, total)
	for _, y := range years {
		for i := 0; i < yearDist[y]; i++ {
			expanded = append(expanded, y)
		}
	}
	median := expanded[len(expanded)/2]

	// Coarse trend: compare paper counts in the first vs second half of the
	// year range (a >20% swing either way is called out as a trend).
	trend := "held roughly steady"
	if maxY > minY {
		mid := minY + (maxY-minY)/2
		var early, late int
		for _, y := range years {
			if y <= mid {
				early += yearDist[y]
			} else {
				late += yearDist[y]
			}
		}
		switch {
		case late > early*12/10:
			trend = "trended upward"
		case early > late*12/10:
			trend = "trended downward"
		}
	}

	return fmt.Sprintf(
		"The corpus spans %d papers published between %d and %d (median publication year: %d). "+
			"Publication volume %s over that period.",
		total, minY, maxY, median, trend,
	), nil
}
