package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (d *DB) SaveDownload(dl *Download) error {
	res, err := d.Exec(`INSERT INTO downloads (paper_id, source, url, status, reason, filename, file_size) VALUES (?,?,?,?,?,?,?)`,
		dl.PaperID, dl.Source, dl.URL, dl.Status, dl.Reason, dl.Filename, dl.FileSize)
	if err != nil {
		return err
	}
	dl.ID, _ = res.LastInsertId()
	return nil
}

func (d *DB) ListDownloads(paperID int64) ([]Download, error) {
	rows, err := d.Query(`SELECT id, paper_id, source, url, status, reason, filename, file_size, attempted_at FROM downloads WHERE paper_id=? ORDER BY attempted_at DESC`, paperID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Download
	for rows.Next() {
		var dl Download
		if err := rows.Scan(&dl.ID, &dl.PaperID, &dl.Source, &dl.URL, &dl.Status, &dl.Reason, &dl.Filename, &dl.FileSize, &dl.AttemptedAt); err != nil {
			return nil, err
		}
		out = append(out, dl)
	}
	return out, rows.Err()
}

// GetPaperPDFPath returns the full path to the downloaded PDF for a paper.
func (d *DB) GetPaperPDFPath(paperID int64) (string, error) {
	var pdfDir, filename string
	err := d.QueryRow(`
		SELECT p.pdf_dir, dl.filename
		FROM downloads dl
		JOIN papers pa ON dl.paper_id = pa.id
		JOIN search_profiles p ON pa.profile_id = p.id
		WHERE dl.paper_id = ? AND dl.status = 'ok' AND dl.filename IS NOT NULL
		ORDER BY dl.attempted_at DESC, dl.id DESC
		LIMIT 1
	`, paperID).Scan(&pdfDir, &filename)
	if err != nil {
		return "", fmt.Errorf("no successful download for paper %d: %w", paperID, err)
	}
	fullPath := filepath.Join(pdfDir, filename)

	// Guard against path traversal: the resolved file must stay inside pdfDir.
	// A malicious or corrupted filename (e.g. "../../etc/passwd") would otherwise
	// let the PDF HTTP handler serve arbitrary files.
	rel, err := filepath.Rel(pdfDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid pdf path for paper %d", paperID)
	}

	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("pdf file not found on disk: %s", fullPath)
	}
	return fullPath, nil
}

func (d *DB) GetDownloadStats(profileID int64) (*DownloadStats, error) {
	stats := &DownloadStats{
		BySource: make(map[string]int),
		ByStatus: make(map[string]int),
	}

	// By source.
	rows, err := d.Query(`SELECT d.source, COUNT(*) FROM downloads d JOIN papers p ON d.paper_id=p.id WHERE p.profile_id=? GROUP BY d.source`, profileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var src string
		var count int
		rows.Scan(&src, &count)
		stats.BySource[src] = count
		stats.TotalAttempts += count
	}
	rows.Close()

	// By status.
	rows, err = d.Query(`SELECT d.status, COUNT(*) FROM downloads d JOIN papers p ON d.paper_id=p.id WHERE p.profile_id=? GROUP BY d.status`, profileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		stats.ByStatus[status] = count
	}
	rows.Close()

	return stats, nil
}

// TopFailReasons returns the most common failure reasons for a profile.
func (d *DB) TopFailReasons(profileID int64, limit int) ([]struct {
	Reason string
	Count  int
}, error) {
	rows, err := d.Query(`SELECT d.source || ': ' || d.reason AS reason, COUNT(*) as cnt FROM downloads d JOIN papers p ON d.paper_id=p.id WHERE p.profile_id=? AND d.status='fail' AND d.reason != '' GROUP BY reason ORDER BY cnt DESC LIMIT ?`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []struct {
		Reason string
		Count  int
	}
	for rows.Next() {
		var r struct {
			Reason string
			Count  int
		}
		rows.Scan(&r.Reason, &r.Count)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetFailedDownloadPaperIDs returns the set of paper IDs within a profile that
// have at least one failed download attempt and NO successful download (no
// filename on disk). Used by the frontend to show a “download failed” badge.
func (d *DB) GetFailedDownloadPaperIDs(profileID int64) (map[int64]bool, error) {
	rows, err := d.Query(`
		SELECT p.id FROM papers p
		WHERE p.profile_id = ?
		  AND EXISTS (SELECT 1 FROM downloads d WHERE d.paper_id = p.id AND d.status = 'fail')
		  AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.paper_id = p.id AND d.status = 'ok' AND d.filename IS NOT NULL)
	`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
