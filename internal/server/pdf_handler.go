package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/qnqatop/papeer/internal/db"
)

var pdfDB *db.DB

// SetPDFDB sets the database reference for serving PDFs.
// Must be called during app startup after the DB is opened.
func SetPDFDB(d *db.DB) {
	pdfDB = d
}

// PDFHandler serves downloaded PDF files from disk via the AssetServer.
// URL pattern: /api/pdf/{paperID}
// It supports HTTP Range requests (handled by http.ServeFile) for large files.
func PDFHandler(w http.ResponseWriter, r *http.Request) {
	if pdfDB == nil {
		http.Error(w, "database not ready", http.StatusServiceUnavailable)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/pdf/")
	// Remove trailing slash if present
	idStr = strings.TrimSuffix(idStr, "/")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid paper ID", http.StatusBadRequest)
		return
	}

	path, err := pdfDB.GetPaperPDFPath(id)
	if err != nil {
		http.Error(w, "PDF not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, path)
}
