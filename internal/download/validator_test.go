package download

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makePDF builds a minimal but valid-looking PDF byte slice.
func makePDF(size int) []byte {
	header := []byte("%PDF-1.4\n")
	trailer := []byte("\n%%EOF\n")

	padLen := size - len(header) - len(trailer)
	if padLen < 0 {
		padLen = 0
	}

	buf := make([]byte, 0, len(header)+padLen+len(trailer))
	buf = append(buf, header...)
	buf = append(buf, make([]byte, padLen)...)
	buf = append(buf, trailer...)
	return buf
}

func TestValidatePDF_Valid(t *testing.T) {
	data := makePDF(25_000)
	if err := ValidatePDF(data); err != nil {
		t.Fatalf("expected valid PDF, got error: %v", err)
	}
}

func TestValidatePDF_Empty(t *testing.T) {
	for _, input := range [][]byte{nil, {}} {
		err := ValidatePDF(input)
		if err == nil {
			t.Fatal("expected error for empty input")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestValidatePDF_BadHeader(t *testing.T) {
	data := make([]byte, 25_000)
	copy(data, []byte("\x00\x01garbage"))
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for bad header")
	}
	if !strings.Contains(err.Error(), "not a PDF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePDF_DetectsHTML(t *testing.T) {
	data := make([]byte, 25_000)
	copy(data, []byte("<html><body>hello</body></html>"))
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for HTML body")
	}
	if !strings.Contains(err.Error(), "HTML") {
		t.Fatalf("expected HTML mention, got: %v", err)
	}
}

func TestValidatePDF_DetectsAccessDenied(t *testing.T) {
	data := make([]byte, 25_000)
	copy(data, []byte("<html><body>Access Denied</body></html>"))
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for access denied")
	}
	if !strings.Contains(err.Error(), "Access Denied") {
		t.Fatalf("expected Access Denied mention, got: %v", err)
	}
}

func TestValidatePDF_DetectsCloudflare(t *testing.T) {
	data := make([]byte, 25_000)
	copy(data, []byte("<html><head><title>Cloudflare</title></head></html>"))
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for cloudflare")
	}
	if !strings.Contains(err.Error(), "Cloudflare") {
		t.Fatalf("expected Cloudflare mention, got: %v", err)
	}
}

func TestValidatePDF_TooSmall(t *testing.T) {
	data := makePDF(1_000)
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for small PDF")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePDF_NoEOF(t *testing.T) {
	data := make([]byte, 25_000)
	copy(data, []byte("%PDF-1.4\n"))
	// no %%EOF at end
	err := ValidatePDF(data)
	if err == nil {
		t.Fatal("expected error for missing EOF marker")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestValidatePDFFile(t *testing.T) {
	data := makePDF(25_000)
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	if err := ValidatePDFFile(path); err != nil {
		t.Fatalf("expected valid PDF file, got error: %v", err)
	}

	// non-existent file
	if err := ValidatePDFFile(filepath.Join(t.TempDir(), "nope.pdf")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
