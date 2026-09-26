package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	gopdf "github.com/ledongthuc/pdf"
)

// Extraction limits: a paper rarely exceeds a few hundred pages, and the LLM
// context can't use millions of characters anyway. They bound CPU/memory on
// hostile or corrupted files.
const (
	maxPDFPages       = 500
	maxExtractedChars = 2_000_000
	pdftotextTimeout  = 60 * time.Second
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

// extractWithGoLib extracts text with ledongthuc/pdf. The library panics on
// many malformed inputs (in NewReader, NumPage, Page), so panics are turned
// into errors and the caller falls back to pdftotext.
func extractWithGoLib(path string) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("pdf parser panic: %v", r)
		}
	}()

	f, r, err := openPDF(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf strings.Builder
	totalPage := min(r.NumPage(), maxPDFPages)
	for pageNum := 1; pageNum <= totalPage && buf.Len() < maxExtractedChars; pageNum++ {
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
	return truncateText(buf.String(), maxExtractedChars), nil
}

// truncateText cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// cappedBuffer keeps the first max bytes written and silently discards the
// rest, so a huge pdftotext output can't exhaust memory.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func extractWithPdftotext(path string) (string, error) {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("pdftotext not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pdftotextTimeout)
	defer cancel()
	out := &cappedBuffer{max: maxExtractedChars}
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-l", fmt.Sprint(maxPDFPages), path, "-")
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return truncateText(out.buf.String(), maxExtractedChars), nil
}
