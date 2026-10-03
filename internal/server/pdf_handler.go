package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// PDFPathPrefix is the URL prefix served by the PDF handler:
// /api/pdf/{paperID}.
const PDFPathPrefix = "/api/pdf/"

// ErrUnavailable is returned by a PDF resolver that can't serve yet (e.g. the
// database is not open); the handler answers 503 instead of 404.
var ErrUnavailable = errors.New("pdf storage not ready")

// PDFResolver maps a paper ID to the absolute path of its validated,
// downloaded PDF.
type PDFResolver func(paperID int64) (string, error)

// NewPDFHandler serves downloaded PDF files from disk via the AssetServer.
// URL pattern: /api/pdf/{paperID}. It supports HTTP Range requests (handled
// by http.ServeFile), so pdf.js can load large files incrementally.
// resolve is responsible for validating the path; a nil resolver or one
// returning ErrUnavailable yields 503, any other resolver error 404.
func NewPDFHandler(resolve PDFResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if resolve == nil {
			http.Error(w, "database not ready", http.StatusServiceUnavailable)
			return
		}

		idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, PDFPathPrefix), "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid paper ID", http.StatusBadRequest)
			return
		}

		path, err := resolve(id)
		if errors.Is(err, ErrUnavailable) {
			http.Error(w, "database not ready", http.StatusServiceUnavailable)
			return
		}
		if err != nil || path == "" {
			http.Error(w, "PDF not found", http.StatusNotFound)
			return
		}

		// Only validated PDFs are stored, so pin the type and forbid sniffing —
		// the webview must never render a stored file as HTML.
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, path)
	})
}
