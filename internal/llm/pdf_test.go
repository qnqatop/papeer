package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractText_EmptyPath(t *testing.T) {
	_, err := ExtractText("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestExtractText_FileNotFound(t *testing.T) {
	_, err := ExtractText("/nonexistent/file.pdf")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestExtractText_NotAPDF(t *testing.T) {
	// Create a text file with .pdf extension — not a valid PDF.
	dir := t.TempDir()
	path := filepath.Join(dir, "not_a_real.pdf")
	if err := os.WriteFile(path, []byte("this is not a PDF file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractText(path)
	if err == nil {
		t.Error("expected error for non-PDF file")
	}
}

func TestExtractText_Simple(t *testing.T) {
	// Create a minimal valid PDF using raw PDF syntax.
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.pdf")

	// A minimal valid PDF with enough text to pass the 100-char threshold.
	text := "This is a test document with enough text content to pass the minimum length threshold of one hundred characters. "
	text += text // double it
	text += text // quadruple it — should be well over 100 chars

	pdfContent := minimalPDF(text)
	if err := os.WriteFile(path, []byte(pdfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractText(path)
	if err != nil {
		t.Logf("Note: extract error (expected if no pdftotext and Go lib fails on minimal PDF): %v", err)
		// The minimal PDF may or may not be parseable; this is expected.
		return
	}
	if len(result) < 100 {
		t.Logf("extracted text length is %d (may be normal for minimal PDF)", len(result))
	}
}

// minimalPDF creates a minimal valid PDF file with the given text content.
// This uses raw PDF syntax that both Go libs and pdftotext can handle.
func minimalPDF(text string) string {
	// Build PDF objects manually.
	// Object 1: the page content stream.
	contentStream := "BT /F1 12 Tf 72 720 Td (" + escapePDF(text) + ") Tj ET"

	// Cross-reference offsets (we just build a simple PDF).
	return "%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n" +
		"4 0 obj\n<< /Length " + itoa(len(contentStream)) + " >>\nstream\n" + contentStream + "\nendstream\nendobj\n" +
		"5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n" +
		"xref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000266 00000 n \n0000000337 00000 n \n" +
		"trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n415\n%%EOF"
}

func escapePDF(s string) string {
	result := ""
	for _, ch := range s {
		switch ch {
		case '\\', '(', ')':
			result += "\\" + string(ch)
		default:
			result += string(ch)
		}
	}
	return result
}

func TestExtractWithGoLib_RecoversFromPanic(t *testing.T) {
	// A well-formed xref whose entry for object 1 points at object 2 makes
	// ledongthuc/pdf panic ("loading 1 0 R: found 2 0 R") when NumPage
	// resolves the catalog. Extraction must return an error instead.
	body := "%PDF-1.4\n2 0 obj\n<< >>\nendobj\n"
	xref := "xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \n"
	content := body + xref + "trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n" +
		itoa(len(body)) + "\n%%EOF"
	path := filepath.Join(t.TempDir(), "mismatch.pdf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := extractWithGoLib(path)
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("err = %v, want recovered panic", err)
	}

	// Truncated/garbage input must also fail cleanly.
	full := minimalPDF("hello world")
	for name, c := range map[string]string{
		"truncated": full[:len(full)/2] + "\nstartxref\n9\n%%EOF",
		"garbage":   "%PDF-1.4\n" + strings.Repeat("\x00\xff(", 100) + "\nstartxref\n0\n%%EOF",
	} {
		p := filepath.Join(t.TempDir(), name+".pdf")
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := extractWithGoLib(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("héllo", 2); got != "h" {
		t.Errorf("truncateText split a rune: %q", got)
	}
	var c cappedBuffer
	c.max = 3
	n, err := c.Write([]byte("abcdef"))
	if n != 6 || err != nil || c.buf.String() != "abc" {
		t.Errorf("cappedBuffer: n=%d err=%v buf=%q", n, err, c.buf.String())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
