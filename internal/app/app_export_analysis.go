package app

import (
	"archive/zip"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/qnqatop/papeer/internal/export"
)

// ExportGapsCSV exports Coverage Gaps (external citations frequently
// referenced by the corpus but not part of it, mention_count >= 2) as CSV.
func (a *App) ExportGapsCSV(profileID int64) (string, error) {
	gaps, err := a.db.GetCoverageGaps(profileID, 2, 1000)
	if err != nil {
		return "", err
	}
	if len(gaps) == 0 {
		return "", fmt.Errorf("no coverage gaps to export — fetch citation data first")
	}

	var buf strings.Builder
	if err := export.ExportGapsCSV(gaps, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ExportAnalysisMarkdown builds a deterministic Markdown report of the
// profile's Analysis section (overview stats, top authors, topics, key
// papers, coverage gaps) — the "report to hand to your supervisor" export.
// Unlike GenerateReviewDraft this makes no LLM calls, so it always works.
func (a *App) ExportAnalysisMarkdown(profileID int64) (string, error) {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return "", err
	}

	stats, err := a.GetStats(profileID, nil)
	if err != nil {
		return "", err
	}
	pdfAvail, err := a.GetPDFAvailability(profileID)
	if err != nil {
		return "", err
	}
	citationStats, err := a.db.GetCitationStats(profileID)
	if err != nil {
		return "", err
	}
	topicList, err := a.GetTopics(profileID, 0)
	if err != nil {
		return "", err
	}
	keyPapers, err := a.GetKeyPapers(profileID)
	if err != nil {
		return "", err
	}
	gaps, _ := a.db.GetCoverageGaps(profileID, 2, 15) // best-effort: may legitimately be empty

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Analysis Report — %s\n\n", profile.Name)

	sb.WriteString("## Overview\n\n")
	fmt.Fprintf(&sb, "- Total papers by status: %s\n", formatCounts(stats.PapersByStatus))
	if yearParagraph, err := a.GenerateYearDistributionParagraph(profileID, nil); err == nil {
		fmt.Fprintf(&sb, "- %s\n", yearParagraph)
	}
	fmt.Fprintf(&sb, "- PDF availability: %d/%d approved papers (%.0f%%)\n", pdfAvail.WithPDF, pdfAvail.Approved, pdfAvail.Percent)
	fmt.Fprintf(&sb, "- Citation graph: %d internal links, %d external works tracked, %d/%d papers processed\n\n",
		citationStats.InternalLinks, citationStats.ExternalPapers, citationStats.PapersProcessed, citationStats.PapersEligible)

	if len(stats.TopAuthors) > 0 {
		sb.WriteString("## Top Authors\n\n")
		for _, au := range stats.TopAuthors {
			fmt.Fprintf(&sb, "- %s (%d papers)\n", au.Name, au.Count)
		}
		sb.WriteString("\n")
	}

	if len(topicList) > 0 {
		sb.WriteString("## Topics\n\n")
		for _, t := range topicList {
			if t.Size == 0 {
				continue
			}
			fmt.Fprintf(&sb, "- **%s** (%d papers) — %s\n", t.Label, t.Size, strings.Join(t.TopTerms, ", "))
		}
		sb.WriteString("\n")
	}

	writeKeyPaperList := func(title string, list []KeyPaper) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintf(&sb, "### %s\n\n", title)
		for _, kp := range list {
			year := "n/a"
			if kp.Year != nil {
				year = fmt.Sprintf("%d", *kp.Year)
			}
			fmt.Fprintf(&sb, "- %s (%s) — score %.2f\n", kp.Title, year, kp.Score)
		}
		sb.WriteString("\n")
	}
	if keyPapers != nil && (len(keyPapers.MostCited) > 0 || len(keyPapers.Rising) > 0 || len(keyPapers.Bridge) > 0) {
		sb.WriteString("## Key Papers\n\n")
		writeKeyPaperList("Most Cited", keyPapers.MostCited)
		writeKeyPaperList("Rising", keyPapers.Rising)
		writeKeyPaperList("Bridge", keyPapers.Bridge)
	}

	if len(gaps) > 0 {
		sb.WriteString("## Coverage Gaps (top 15)\n\n")
		for _, g := range gaps {
			year := "n/a"
			if g.Year != nil {
				year = fmt.Sprintf("%d", *g.Year)
			}
			fmt.Fprintf(&sb, "- %s (%s) — mentioned by %d paper(s)\n", g.Title, year, g.MentionCount)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func formatCounts(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, status := range []string{"new", "approved", "downloaded", "rejected"} {
		if c, ok := m[status]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d", status, c))
		}
	}
	return strings.Join(parts, ", ")
}

// SaveAnalysisBundle opens a save dialog and writes a single .zip containing
// the Coverage Gaps CSV, the deterministic Analysis Markdown report, and
// (when supplied by the caller) the LLM review draft and a citation-graph
// PNG. reviewMarkdown and graphPNGBase64 may be empty — the graph PNG in
// particular can only be rendered client-side (canvas/SVG export), so the
// frontend passes it in as base64. Returns the saved file path, or "" if the
// user cancelled the dialog.
func (a *App) SaveAnalysisBundle(profileID int64, reviewMarkdown string, graphPNGBase64 string) (string, error) {
	analysisMD, err := a.ExportAnalysisMarkdown(profileID)
	if err != nil {
		return "", err
	}
	gapsCSV, err := a.ExportGapsCSV(profileID)
	if err != nil {
		gapsCSV = "" // no gaps yet — skip that entry rather than failing the whole bundle
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save Analysis Bundle",
		DefaultFilename: "analysis-bundle.zip",
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP Archive", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // cancelled
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create bundle: %w", err)
	}
	// On any failure, don't leave a truncated/corrupt zip behind.
	written := false
	defer func() {
		if !written {
			f.Close()
			os.Remove(path)
		}
	}()

	zw := zip.NewWriter(f)
	writeEntry := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	if err := writeEntry("analysis.md", []byte(analysisMD)); err != nil {
		zw.Close()
		return "", err
	}
	if gapsCSV != "" {
		if err := writeEntry("coverage_gaps.csv", []byte(gapsCSV)); err != nil {
			zw.Close()
			return "", err
		}
	}
	if strings.TrimSpace(reviewMarkdown) != "" {
		if err := writeEntry("review_draft.md", []byte(reviewMarkdown)); err != nil {
			zw.Close()
			return "", err
		}
	}
	if graphPNGBase64 != "" {
		raw, derr := base64.StdEncoding.DecodeString(graphPNGBase64)
		if derr != nil {
			runtime.LogWarningf(a.ctx, "bundle: decode graph png: %v", derr)
		} else if err := writeEntry("citation_graph.png", raw); err != nil {
			zw.Close()
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("finalize bundle: %w", err)
	}
	written = true
	// Close flushes the file; an error here means the zip may be incomplete.
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("write bundle: %w", err)
	}
	return path, nil
}
