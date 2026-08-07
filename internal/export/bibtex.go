package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

// ExportBibTeX writes papers as BibTeX entries to w.
func ExportBibTeX(papers []db.Paper, w io.Writer) error {
	for _, p := range papers {
		key := bibtexKey(p)
		if _, err := fmt.Fprintf(w, "@article{%s,\n", key); err != nil {
			return err
		}
		if p.Title != "" {
			if err := writeBibField(w, "title", p.Title); err != nil {
				return err
			}
		}
		if len(p.Authors) > 0 {
			if err := writeBibField(w, "author", strings.Join(p.Authors, " and ")); err != nil {
				return err
			}
		}
		if p.Year != nil {
			if err := writeBibField(w, "year", fmt.Sprintf("%d", *p.Year)); err != nil {
				return err
			}
		}
		if p.Venue != "" {
			if err := writeBibField(w, "journal", p.Venue); err != nil {
				return err
			}
		}
		if doi := ptr.Val(p.DOI); doi != "" {
			if err := writeBibField(w, "doi", doi); err != nil {
				return err
			}
		}
		if u := ptr.Val(p.PdfURL); u != "" {
			if err := writeBibField(w, "url", u); err != nil {
				return err
			}
		}
		if p.Abstract != "" {
			if err := writeBibField(w, "abstract", p.Abstract); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "}"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func writeBibField(w io.Writer, name, value string) error {
	_, err := fmt.Fprintf(w, "  %s = {%s},\n", name, value)
	return err
}

func bibtexKey(p db.Paper) string {
	var parts []string

	if len(p.Authors) > 0 {
		// Take last name of first author (first word before comma or last word).
		author := p.Authors[0]
		fields := strings.Fields(author)
		if len(fields) > 0 {
			last := strings.TrimRight(fields[len(fields)-1], ",.")
			parts = append(parts, sanitizeKey(last))
		}
	}

	if p.Year != nil {
		parts = append(parts, fmt.Sprintf("%d", *p.Year))
	}

	if p.Title != "" {
		words := strings.Fields(p.Title)
		if len(words) > 0 {
			parts = append(parts, sanitizeKey(words[0]))
		}
	}

	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, "_")
}

func sanitizeKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
