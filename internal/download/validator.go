package download

import (
	"bytes"
	"fmt"
	"os"
)

const (
	pdfMinSize    = 20_000
	pdfEOFTailLen = 2048
)

var (
	pdfHeader = []byte("%PDF-")
	pdfEOF    = []byte("%%EOF")
)

// ValidatePDF checks that data looks like a complete, non-stub PDF.
// Returns nil when valid; a descriptive error otherwise.
func ValidatePDF(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty response")
	}

	if len(data) < len(pdfHeader) || !bytes.Equal(data[:len(pdfHeader)], pdfHeader) {
		// Inspect the first kilobyte for common HTML/error markers so the log
		// shows a useful cause instead of a raw header byte dump.
		head := data
		if len(head) > 1024 {
			head = head[:1024]
		}
		low := bytes.ToLower(head)
		switch {
		case bytes.Contains(low, []byte("access denied")):
			return fmt.Errorf("publisher returned HTML \"Access Denied\" (likely bot/IP block)")
		case bytes.Contains(low, []byte("cloudflare")):
			return fmt.Errorf("blocked by Cloudflare challenge")
		case bytes.Contains(low, []byte("captcha")):
			return fmt.Errorf("captcha required")
		case bytes.Contains(low, []byte("<!doctype html")) || bytes.Contains(low, []byte("<html")):
			return fmt.Errorf("got HTML page instead of PDF (no embedded PDF link found)")
		}
		preview := head
		if len(preview) > 8 {
			preview = preview[:8]
		}
		return fmt.Errorf("not a PDF (header=%q)", preview)
	}

	if len(data) < pdfMinSize {
		return fmt.Errorf("too small (%d bytes — likely paywall stub)", len(data))
	}

	tail := data
	if len(tail) > pdfEOFTailLen {
		tail = tail[len(tail)-pdfEOFTailLen:]
	}
	if !bytes.Contains(tail, pdfEOF) {
		return fmt.Errorf("PDF truncated (no %%%%EOF marker)")
	}

	return nil
}

// ValidatePDFFile reads a file from disk and validates it as a PDF.
func ValidatePDFFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	return ValidatePDF(data)
}
