package search

import (
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9а-яё ]+`)
var multiSpace = regexp.MustCompile(`\s+`)

// NormalizeTitle lowercases and strips non-alphanumeric chars for dedup matching.
func NormalizeTitle(t string) string {
	t = strings.ToLower(t)
	t = nonAlphaNum.ReplaceAllString(t, " ")
	t = multiSpace.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// sourcePriority defines PDF URL priority (lower = better).
var pdfSourcePriority = map[string]int{
	"arxiv": 0, "semantic_scholar": 1, "openalex": 2,
	"unpaywall": 3, "crossref": 4, "s2_title": 5,
	"cyberleninka": 6,
}

// Dedupe merges raw papers by DOI (exact) then normalized title.
// Combines sources, keeps best PDF URL, longest abstract, max citations.
func Dedupe(records []RawPaper) []RawPaper {
	type merged struct {
		RawPaper
		sources map[string]bool
		pdfPrio int
	}

	byKey := make(map[string]*merged)
	var order []string // preserve insertion order

	for _, r := range records {
		if strings.TrimSpace(r.Title) == "" {
			continue
		}

		titleKey := "title:" + NormalizeTitle(r.Title)
		doiKey := ""
		if r.DOI != "" {
			doiKey = "doi:" + r.DOI
		}

		// Try to find existing entry by DOI first, then by title.
		var existing *merged
		var existingKey string
		if doiKey != "" {
			if m, ok := byKey[doiKey]; ok {
				existing = m
				existingKey = doiKey
			}
		}
		if existing == nil {
			if m, ok := byKey[titleKey]; ok {
				existing = m
				existingKey = titleKey
			}
		}

		if existing != nil {
			// Merge into existing.
			existing.sources[r.Source] = true

			if r.CitationCount > existing.CitationCount {
				existing.CitationCount = r.CitationCount
			}
			if len(r.Abstract) > len(existing.Abstract) {
				existing.Abstract = r.Abstract
			}

			// Better PDF source.
			if r.PdfURL != "" {
				newPrio := pdfSourcePriority[r.Source]
				if existing.PdfURL == "" || newPrio < existing.pdfPrio {
					existing.PdfURL = r.PdfURL
					existing.pdfPrio = newPrio
				}
			}

			// Fill missing fields.
			if existing.DOI == "" && r.DOI != "" {
				existing.DOI = r.DOI
			}
			if existing.ArxivID == "" && r.ArxivID != "" {
				existing.ArxivID = r.ArxivID
			}
			if existing.Venue == "" && r.Venue != "" {
				existing.Venue = r.Venue
			}
			if existing.Year == nil && r.Year != nil {
				existing.Year = r.Year
			}

			// If we gained a DOI, also register under DOI key for future lookups.
			if doiKey != "" && existingKey != doiKey {
				byKey[doiKey] = existing
			}
			// Also register under title key if not already.
			if _, ok := byKey[titleKey]; !ok {
				byKey[titleKey] = existing
			}
		} else {
			prio := 99
			if r.PdfURL != "" {
				prio = pdfSourcePriority[r.Source]
			}
			m := &merged{
				RawPaper: r,
				sources:  map[string]bool{r.Source: true},
				pdfPrio:  prio,
			}
			// Register under both keys.
			if doiKey != "" {
				byKey[doiKey] = m
			}
			byKey[titleKey] = m
			order = append(order, titleKey)
		}
	}

	out := make([]RawPaper, 0, len(order))
	seen := make(map[*merged]bool)
	for _, key := range order {
		m := byKey[key]
		if seen[m] {
			continue
		}
		seen[m] = true
		// Flatten sources into the Source field as comma-separated for RawPaper.
		// The engine will split this into a proper slice when converting to Paper.
		sources := make([]string, 0, len(m.sources))
		for s := range m.sources {
			sources = append(sources, s)
		}
		m.RawPaper.Source = strings.Join(sources, ",")
		out = append(out, m.RawPaper)
	}
	return out
}
