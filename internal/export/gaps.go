package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/qnqatop/papeer/internal/db"
)

// ExportGapsCSV writes Coverage Gaps (external citations frequently
// referenced by the corpus but not part of it) as CSV to w.
func ExportGapsCSV(gaps []db.ExternalCitationWithMentions, w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"title", "year", "authors", "citation_count", "mention_count",
		"mentioned_by_paper_ids", "s2_paper_id",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, g := range gaps {
		year := ""
		if g.Year != nil {
			year = fmt.Sprintf("%d", *g.Year)
		}
		ids := make([]string, len(g.MentionedBy))
		for i, id := range g.MentionedBy {
			ids[i] = strconv.FormatInt(id, 10)
		}
		row := []string{
			g.Title,
			year,
			strings.Join(g.Authors, "; "),
			strconv.Itoa(g.CitationCount),
			strconv.Itoa(g.MentionCount),
			strings.Join(ids, "; "),
			g.S2PaperID,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	return nil
}
