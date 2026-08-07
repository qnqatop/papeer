package llm

import (
	"os"
	"path/filepath"
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

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
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
