package llm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	gopdf "github.com/ledongthuc/pdf"
)

// ExtractText extracts text from a PDF file.
// Strategy: Go library (ledongthuc/pdf) → pdftotext → error.
func ExtractText(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %s", path)
	}

	text, err := extractWithGoLib(path)
	if err == nil && len(strings.TrimSpace(text)) >= 100 {
		return text, nil
	}

	if _, lookErr := exec.LookPath("pdftotext"); lookErr != nil {
		// The Go library already failed (scanned/complex PDF) and pdftotext is
		// not installed — point the user at the fix.
		return "", fmt.Errorf("could not extract text: this PDF needs pdftotext, which is not installed. " +
			"Install it with `brew install poppler` (macOS) or `apt install poppler-utils` (Linux)")
	}

	text, err = extractWithPdftotext(path)
	if err == nil && len(strings.TrimSpace(text)) >= 100 {
		return text, nil
	}

	return "", fmt.Errorf("failed to extract text from PDF (tried go lib and pdftotext)")
}

func openPDF(path string) (*os.File, *gopdf.Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	r, err := gopdf.NewReader(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, r, nil
}

func extractWithGoLib(path string) (string, error) {
	f, r, err := openPDF(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf strings.Builder
	totalPage := r.NumPage()
	for pageNum := 1; pageNum <= totalPage; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

func extractWithPdftotext(path string) (string, error) {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("pdftotext not found: %w", err)
	}

	cmd := exec.Command("pdftotext", "-layout", path, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(out), nil
}
