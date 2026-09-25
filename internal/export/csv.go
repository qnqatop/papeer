package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

// csvSafe neutralises spreadsheet formula injection: text cells taken from
// remote metadata (titles, authors, venues…) that start with a formula
// trigger character are prefixed with a single quote so Excel/LibreOffice
// treat them as plain text instead of evaluating them.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// ExportCSV writes papers as CSV to w.
func ExportCSV(papers []db.Paper, w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"title", "authors", "year", "venue", "doi",
		"arxiv_id", "pdf_url", "citations", "score", "status", "sources",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, p := range papers {
		year := ""
		if p.Year != nil {
			year = fmt.Sprintf("%d", *p.Year)
		}
		row := []string{
			csvSafe(p.Title),
			csvSafe(strings.Join(p.Authors, "; ")),
			year,
			csvSafe(p.Venue),
			csvSafe(ptr.Val(p.DOI)),
			csvSafe(ptr.Val(p.ArxivID)),
			csvSafe(ptr.Val(p.PdfURL)),
			fmt.Sprintf("%d", p.CitationCount),
			fmt.Sprintf("%d", p.PreScore),
			csvSafe(p.Status),
			csvSafe(strings.Join(p.Sources, "; ")),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	return nil
}
