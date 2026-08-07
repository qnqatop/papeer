package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

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
			p.Title,
			strings.Join(p.Authors, "; "),
			year,
			p.Venue,
			ptr.Val(p.DOI),
			ptr.Val(p.ArxivID),
			ptr.Val(p.PdfURL),
			fmt.Sprintf("%d", p.CitationCount),
			fmt.Sprintf("%d", p.PreScore),
			p.Status,
			strings.Join(p.Sources, "; "),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	return nil
}
