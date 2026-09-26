package export

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
)

func TestCSVSafe(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"Plain title":         "Plain title",
		"=HYPERLINK(\"x\")":   "'=HYPERLINK(\"x\")",
		"+1+2":                "'+1+2",
		"-2+3":                "'-2+3",
		"@SUM(A1)":            "'@SUM(A1)",
		"\tcmd":               "'\tcmd",
		"\rcmd":               "'\rcmd",
		"a=b stays":           "a=b stays",
		"Über-title (ümlaut)": "Über-title (ümlaut)",
	}
	for in, want := range cases {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExportCSV_NeutralisesFormulas(t *testing.T) {
	papers := []db.Paper{{
		Title:   "=cmd|' /C calc'!A0",
		Authors: db.JSONStringSlice{"@evil", "Bob"},
		Venue:   "+venue",
		Year:    ptr.Ptr(2020),
	}}
	var buf bytes.Buffer
	if err := ExportCSV(papers, &buf); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	row := recs[1]
	if row[0] != "'=cmd|' /C calc'!A0" || row[1] != "'@evil; Bob" || row[3] != "'+venue" {
		t.Errorf("formula cells not neutralised: %q", row)
	}
	if row[2] != "2020" {
		t.Errorf("year = %q, want 2020", row[2])
	}
}

func TestExportGapsCSV_NeutralisesFormulas(t *testing.T) {
	gaps := []db.ExternalCitationWithMentions{}
	g := db.ExternalCitationWithMentions{}
	g.Title = "-1+1"
	g.Authors = db.JSONStringSlice{"=A1"}
	gaps = append(gaps, g)

	var buf bytes.Buffer
	if err := ExportGapsCSV(gaps, &buf); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if recs[1][0] != "'-1+1" || recs[1][2] != "'=A1" {
		t.Errorf("gaps formula cells not neutralised: %q", recs[1])
	}
}

func TestExportBibTeX_EscapesSpecialChars(t *testing.T) {
	papers := []db.Paper{{
		Title:    "50% of R&D costs $5 #1 a_b ~x^2 \\cmd",
		Authors:  db.JSONStringSlice{"O'Brien {Jr}"},
		Abstract: "broken } injected = {evil},\n  note = {pwned",
		DOI:      ptr.Ptr("10.1/a_b{c}"),
		Year:     ptr.Ptr(2021),
	}}
	var buf bytes.Buffer
	if err := ExportBibTeX(papers, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	wantTitle := `title = {50\% of R\&D costs \$5 \#1 a\_b \textasciitilde{}x\textasciicircum{}2 \textbackslash{}cmd},`
	if !strings.Contains(out, wantTitle) {
		t.Errorf("title not escaped:\n%s", out)
	}
	if !strings.Contains(out, `author = {O'Brien \{Jr\}},`) {
		t.Errorf("author not escaped:\n%s", out)
	}
	if !strings.Contains(out, `doi = {10.1/a_b%7Bc%7D},`) {
		t.Errorf("doi not escaped:\n%s", out)
	}

	// Every unescaped brace must be balanced: the injected "note" must stay
	// inside the abstract value.
	depth := 0
	for i := 0; i < len(out); i++ {
		switch out[i] {
		case '\\':
			i++ // skip escaped char
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				t.Fatalf("unbalanced braces at %d:\n%s", i, out)
			}
		}
	}
	if depth != 0 {
		t.Errorf("unbalanced braces (depth %d):\n%s", depth, out)
	}
}

func TestExportBibTeX_UniqueKeys(t *testing.T) {
	p := db.Paper{Title: "Deep things", Authors: db.JSONStringSlice{"Alice Smith"}, Year: ptr.Ptr(2021)}
	var buf bytes.Buffer
	if err := ExportBibTeX([]db.Paper{p, p, p}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, key := range []string{"@article{smith_2021_deep,", "@article{smith_2021_deepa,", "@article{smith_2021_deepb,"} {
		if strings.Count(out, key) != 1 {
			t.Errorf("expected exactly one %q in:\n%s", key, out)
		}
	}
}

func TestKeySuffix(t *testing.T) {
	for n, want := range map[int]string{0: "a", 25: "z", 26: "aa", 27: "ab", 51: "az", 52: "ba"} {
		if got := keySuffix(n); got != want {
			t.Errorf("keySuffix(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestYAML_ExcludeKeywordsRoundTrip(t *testing.T) {
	axes := []db.Axis{{
		AxisKey: "t",
		Keywords: []db.Keyword{
			{Word: "m", Type: "must"},
			{Word: "b", Type: "boost"},
			{Word: "survey", Type: "exclude"},
		},
	}}
	data, err := ExportAxesToYAML(axes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "keywords_exclude:") {
		t.Errorf("exported yaml lacks keywords_exclude:\n%s", data)
	}
	got, err := ImportAxesFromYAML(bytes.NewReader(data), 1)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]string{}
	for _, k := range got[0].Keywords {
		types[k.Word] = k.Type
	}
	if types["survey"] != "exclude" || types["m"] != "must" || types["b"] != "boost" {
		t.Errorf("keywords after round trip = %v", got[0].Keywords)
	}
}

func TestImportAxesFromYAML_SortedOrder(t *testing.T) {
	yml := "topics:\n  zeta: {description: z}\n  alpha: {description: a}\n  mid: {description: m}\n"
	for i := 0; i < 5; i++ { // map order is random; repeat to catch it
		axes, err := ImportAxesFromYAML(strings.NewReader(yml), 1)
		if err != nil {
			t.Fatal(err)
		}
		if axes[0].AxisKey != "alpha" || axes[1].AxisKey != "mid" || axes[2].AxisKey != "zeta" {
			t.Fatalf("order = %s,%s,%s", axes[0].AxisKey, axes[1].AxisKey, axes[2].AxisKey)
		}
		for j, a := range axes {
			if a.Position != j {
				t.Errorf("axis %s position = %d, want %d", a.AxisKey, a.Position, j)
			}
		}
	}
}

func TestImportAxesFromYAML_SizeLimitAndEmpty(t *testing.T) {
	big := strings.Repeat("#", MaxYAMLImportSize+1)
	if _, err := ImportAxesFromYAML(strings.NewReader(big), 1); !errors.Is(err, ErrYAMLTooLarge) {
		t.Errorf("oversized input err = %v, want ErrYAMLTooLarge", err)
	}
	if _, err := ImportAxesFromYAML(strings.NewReader("  \n"), 1); err == nil {
		t.Error("empty input must be an error")
	}
}
