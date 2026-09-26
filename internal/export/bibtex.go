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
	usedKeys := make(map[string]bool, len(papers))
	for _, p := range papers {
		key := uniqueKey(bibtexKey(p), usedKeys)
		if _, err := fmt.Fprintf(w, "@article{%s,\n", key); err != nil {
			return err
		}
		if p.Title != "" {
			if err := writeBibField(w, "title", p.Title); err != nil {
				return err
			}
		}
		if len(p.Authors) > 0 {
			// Escape each name separately: " and " is BibTeX syntax.
			authors := make([]string, len(p.Authors))
			for i, a := range p.Authors {
				authors[i] = bibEscaper.Replace(a)
			}
			if err := writeRawBibField(w, "author", strings.Join(authors, " and ")); err != nil {
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
		// doi/url are verbatim fields: LaTeX escapes would show up literally,
		// so only brace-breaking characters are percent-encoded.
		if doi := ptr.Val(p.DOI); doi != "" {
			if err := writeRawBibField(w, "doi", verbatimEscaper.Replace(doi)); err != nil {
				return err
			}
		}
		if u := ptr.Val(p.PdfURL); u != "" {
			if err := writeRawBibField(w, "url", verbatimEscaper.Replace(u)); err != nil {
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

// bibEscaper escapes LaTeX special characters in free-text field values, so
// remote metadata (e.g. an abstract with an unbalanced "}") can neither break
// the entry nor inject extra fields.
var bibEscaper = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`{`, `\{`,
	`}`, `\}`,
	`%`, `\%`,
	`&`, `\&`,
	`$`, `\$`,
	`#`, `\#`,
	`_`, `\_`,
	`~`, `\textasciitilde{}`,
	`^`, `\textasciicircum{}`,
)

// verbatimEscaper keeps doi/url values brace-balanced without LaTeX escapes.
var verbatimEscaper = strings.NewReplacer(`\`, `%5C`, `{`, `%7B`, `}`, `%7D`)

// writeBibField writes a free-text field, escaping its value.
func writeBibField(w io.Writer, name, value string) error {
	return writeRawBibField(w, name, bibEscaper.Replace(value))
}

// writeRawBibField writes a field whose value is already escaped.
func writeRawBibField(w io.Writer, name, value string) error {
	_, err := fmt.Fprintf(w, "  %s = {%s},\n", name, value)
	return err
}

// uniqueKey returns key, or key with a letter suffix (a, b, c, …) if it was
// already used in this export, and records the result in used.
func uniqueKey(key string, used map[string]bool) string {
	candidate := key
	for n := 0; used[candidate]; n++ {
		candidate = key + keySuffix(n)
	}
	used[candidate] = true
	return candidate
}

// keySuffix maps 0→"a", 25→"z", 26→"aa", 27→"ab", …
func keySuffix(n int) string {
	s := ""
	for n >= 0 {
		s = string(rune('a'+n%26)) + s
		n = n/26 - 1
	}
	return s
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
