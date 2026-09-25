package download

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/qnqatop/papeer/internal/httpclient"
)

func TestIsBotWallError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("all UA strategies failed: %w", &httpclient.StatusError{Code: 403}), true},
		{&httpclient.StatusError{Code: 429}, true},
		{&httpclient.StatusError{Code: 503}, true},
		{&httpclient.StatusError{Code: 404}, false},
		{errors.New(`Get "file:///etc/passwd": unsupported protocol scheme "file"`), false},
		{errors.New("dial tcp: connection refused"), false},
	}
	for _, tc := range cases {
		if got := isBotWallError(tc.err); got != tc.want {
			t.Errorf("isBotWallError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
	if isHTTPURL("file:///etc/passwd") || isHTTPURL("ftp://x/y.pdf") || !isHTTPURL("https://x.org/a.pdf") {
		t.Error("isHTTPURL misclassified")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(dest, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "new" {
		t.Errorf("content = %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
	if err := writeFileAtomic(filepath.Join(dir, "missing", "b.pdf"), []byte("x")); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestExtractPDF_MetaTag(t *testing.T) {
	html := `<html><head>
		<meta name="citation_pdf_url" content="https://example.com/paper.pdf">
	</head><body></body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/article")
	want := "https://example.com/paper.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_HrefLink(t *testing.T) {
	html := `<html><body>
		<a href="/papers/test.pdf?token=abc">Download</a>
	</body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/article")
	want := "https://example.com/papers/test.pdf?token=abc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_EmbedTag(t *testing.T) {
	html := `<html><body>
		<embed src="//cdn.example.com/doc.pdf" type="application/pdf">
	</body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/view")
	want := "https://cdn.example.com/doc.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_IframeTag(t *testing.T) {
	html := `<html><body>
		<iframe src="/viewer/paper.pdf" width="100%"></iframe>
	</body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/page")
	want := "https://example.com/viewer/paper.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_NoPDF(t *testing.T) {
	html := `<html><body><p>This is a plain HTML page with no PDF links.</p></body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/page")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractPDF_RelativeURL(t *testing.T) {
	html := `<html><body>
		<a href="../downloads/paper.pdf">PDF</a>
	</body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/articles/123")
	want := "https://example.com/downloads/paper.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_MetaTagContentFirst(t *testing.T) {
	// MDPI/Springer ordering: content="..." comes before name="citation_pdf_url".
	html := `<html><head>
		<meta content="https://www.mdpi.com/1424-8220/22/4/1410/pdf" name="citation_pdf_url" />
	</head></html>`

	got := ExtractPDFFromHTML(html, "https://www.mdpi.com/1424-8220/22/4/1410")
	want := "https://www.mdpi.com/1424-8220/22/4/1410/pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_MetaRefresh(t *testing.T) {
	html := `<html><head>
		<meta http-equiv="refresh" content="3; url=/downloads/final.pdf">
	</head></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/wait")
	want := "https://example.com/downloads/final.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractPDF_JSLocation(t *testing.T) {
	html := `<html><body>
		<script>setTimeout(function(){ window.location.href = "https://cdn.example.com/x.pdf?v=1"; }, 4000);</script>
	</body></html>`

	got := ExtractPDFFromHTML(html, "https://example.com/wait")
	want := "https://cdn.example.com/x.pdf?v=1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsPDFEndpoint(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.mdpi.com/1424-8220/22/4/1410/pdf?version=1644651882", true},
		{"https://example.com/papers/x.pdf", true},
		{"https://example.com/pdf/123", true},
		{"https://example.com/article/123", false},
		{"https://example.com/", false},
	}
	for _, c := range cases {
		if got := isPDFEndpoint(c.url); got != c.want {
			t.Errorf("isPDFEndpoint(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
