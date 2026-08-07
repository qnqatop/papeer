package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UpsertCitationLink inserts a citation link (from_paper_id → to_paper_id) if not exists.
func (d *DB) UpsertCitationLink(profileID, fromPaperID, toPaperID int64) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO citation_links (profile_id, from_paper_id, to_paper_id) VALUES (?,?,?)`,
		profileID, fromPaperID, toPaperID)
	return err
}

// GetCitationLinks returns all citation links for a profile.
func (d *DB) GetCitationLinks(profileID int64) ([]CitationLink, error) {
	rows, err := d.Query(`SELECT id, profile_id, from_paper_id, to_paper_id FROM citation_links WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []CitationLink
	for rows.Next() {
		var l CitationLink
		if err := rows.Scan(&l.ID, &l.ProfileID, &l.FromPaperID, &l.ToPaperID); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// UpsertExternalCitation inserts or increments mention_count for an external citation.
func (d *DB) UpsertExternalCitation(profileID int64, ec *ExternalCitation) error {
	_, err := d.upsertExternalCitation(profileID, ec)
	return err
}

// UpsertExternalCitationGetID behaves like UpsertExternalCitation but also
// returns the row's id, so callers can link a citation_mentions row to it.
func (d *DB) UpsertExternalCitationGetID(profileID int64, ec *ExternalCitation) (int64, error) {
	return d.upsertExternalCitation(profileID, ec)
}

func (d *DB) upsertExternalCitation(profileID int64, ec *ExternalCitation) (int64, error) {
	authorsJSON, err := json.Marshal(ec.Authors)
	if err != nil {
		authorsJSON = []byte("[]")
	}

	var id int64
	err = d.QueryRow(`INSERT INTO external_citations (profile_id, s2_paper_id, title, year, citation_count, authors, mention_count)
		VALUES (?,?,?,?,?,?,1)
		ON CONFLICT(profile_id, s2_paper_id) DO UPDATE SET
			mention_count = mention_count + 1,
			citation_count = MAX(citation_count, excluded.citation_count),
			title = CASE WHEN LENGTH(excluded.title) > LENGTH(title) THEN excluded.title ELSE title END
		RETURNING id`,
		profileID, ec.S2PaperID, ec.Title, ec.Year, ec.CitationCount, string(authorsJSON)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert external citation %q: %w", ec.S2PaperID, err)
	}
	return id, nil
}

// UpsertCitationMention records that paperID mentions (references, or is
// referenced by) the given external citation. Idempotent.
func (d *DB) UpsertCitationMention(paperID, externalID int64) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO citation_mentions (paper_id, external_id) VALUES (?,?)`, paperID, externalID)
	if err != nil {
		return fmt.Errorf("upsert citation mention (paper=%d, external=%d): %w", paperID, externalID, err)
	}
	return nil
}

// GetCoverageGaps returns external citations (mention_count >= minMentions)
// sorted by mention_count desc, each annotated with the internal paper IDs
// that mention it. This is the data behind the "Coverage Gaps" tab.
func (d *DB) GetCoverageGaps(profileID int64, minMentions, limit int) ([]ExternalCitationWithMentions, error) {
	externals, err := d.GetExternalCitations(profileID, minMentions, limit)
	if err != nil {
		return nil, err
	}
	if len(externals) == 0 {
		return nil, nil
	}

	ids := make([]int64, len(externals))
	idx := make(map[int64]int, len(externals))
	for i, ec := range externals {
		ids[i] = ec.ID
		idx[ec.ID] = i
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := d.Query(fmt.Sprintf(
		`SELECT external_id, paper_id FROM citation_mentions WHERE external_id IN (%s)`,
		strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ExternalCitationWithMentions, len(externals))
	for i, ec := range externals {
		out[i] = ExternalCitationWithMentions{ExternalCitation: ec}
	}
	for rows.Next() {
		var extID, paperID int64
		if err := rows.Scan(&extID, &paperID); err != nil {
			return nil, err
		}
		if i, ok := idx[extID]; ok {
			out[i].MentionedBy = append(out[i].MentionedBy, paperID)
		}
	}
	return out, rows.Err()
}

// GetExternalCitations returns external citations sorted by mention_count desc.
func (d *DB) GetExternalCitations(profileID int64, minMentions int, limit int) ([]ExternalCitation, error) {
	rows, err := d.Query(`SELECT id, profile_id, s2_paper_id, title, year, citation_count, authors, mention_count
		FROM external_citations
		WHERE profile_id=? AND mention_count>=?
		ORDER BY mention_count DESC, citation_count DESC
		LIMIT ?`, profileID, minMentions, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ExternalCitation
	for rows.Next() {
		var ec ExternalCitation
		if err := rows.Scan(&ec.ID, &ec.ProfileID, &ec.S2PaperID, &ec.Title, &ec.Year,
			&ec.CitationCount, &ec.Authors, &ec.MentionCount); err != nil {
			return nil, err
		}
		result = append(result, ec)
	}
	return result, rows.Err()
}

// GetExternalCitationByID fetches a single external citation by its row id,
// scoped to a profile. Used when promoting a Coverage Gaps entry to a paper.
func (d *DB) GetExternalCitationByID(profileID, id int64) (*ExternalCitation, error) {
	var ec ExternalCitation
	err := d.QueryRow(`SELECT id, profile_id, s2_paper_id, title, year, citation_count, authors, mention_count
		FROM external_citations WHERE id=? AND profile_id=?`, id, profileID).
		Scan(&ec.ID, &ec.ProfileID, &ec.S2PaperID, &ec.Title, &ec.Year, &ec.CitationCount, &ec.Authors, &ec.MentionCount)
	if err != nil {
		return nil, fmt.Errorf("external citation %d: %w", id, err)
	}
	return &ec, nil
}

// FindPaperByS2ID finds a paper by Semantic Scholar paper ID.
func (d *DB) FindPaperByS2ID(profileID int64, s2ID string) (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM papers WHERE profile_id=? AND s2_paper_id=?`, profileID, s2ID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// FindPaperByDOI finds a paper by DOI.
func (d *DB) FindPaperByDOI(profileID int64, doi string) (int64, error) {
	var id int64
	err := d.QueryRow(`SELECT id FROM papers WHERE profile_id=? AND doi=?`, profileID, doi).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// SetPaperS2ID sets the Semantic Scholar paper ID for a paper.
func (d *DB) SetPaperS2ID(paperID int64, s2ID string) error {
	_, err := d.Exec(`UPDATE papers SET s2_paper_id=? WHERE id=?`, s2ID, paperID)
	return err
}

// SetLastCitationFetchAt marks a paper as processed by the citation-fetch
// worker (whether or not a match was found). Used for resume and for the
// accurate papers_processed stat.
func (d *DB) SetLastCitationFetchAt(paperID int64, t time.Time) error {
	_, err := d.Exec(`UPDATE papers SET last_citation_fetch_at=? WHERE id=?`, t, paperID)
	if err != nil {
		return fmt.Errorf("set last_citation_fetch_at for paper %d: %w", paperID, err)
	}
	return nil
}

// ClearCitationData deletes all citation data for a profile and resets the
// per-paper fetch-progress marker so a subsequent FetchCitations starts
// fresh instead of skipping everything as "already processed". Runs in a
// single transaction so a mid-way failure can't leave the graph half-wiped.
func (d *DB) ClearCitationData(profileID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM citation_links WHERE profile_id=?`, profileID); err != nil {
		return fmt.Errorf("delete citation_links: %w", err)
	}
	// citation_mentions cascades via FK when external_citations rows are
	// deleted (foreign_keys=ON), but only for the exact rows removed below.
	if _, err := tx.Exec(`DELETE FROM external_citations WHERE profile_id=?`, profileID); err != nil {
		return fmt.Errorf("delete external_citations: %w", err)
	}
	if _, err := tx.Exec(`UPDATE papers SET last_citation_fetch_at=NULL WHERE profile_id=?`, profileID); err != nil {
		return fmt.Errorf("reset last_citation_fetch_at: %w", err)
	}

	return tx.Commit()
}

// CitationStats holds statistics about the citation graph.
type CitationStats struct {
	InternalLinks   int `json:"internal_links"`
	ExternalPapers  int `json:"external_papers"`
	PapersProcessed int `json:"papers_processed"`
	// PapersEligible is the count of approved/downloaded papers that are
	// candidates for citation fetch (used to show "processed / eligible").
	PapersEligible int `json:"papers_eligible"`
}

// GetCitationStats returns statistics about the citation graph.
func (d *DB) GetCitationStats(profileID int64) (*CitationStats, error) {
	var s CitationStats

	err := d.QueryRow(`SELECT COUNT(*) FROM citation_links WHERE profile_id=?`, profileID).Scan(&s.InternalLinks)
	if err != nil {
		return nil, err
	}

	err = d.QueryRow(`SELECT COUNT(*) FROM external_citations WHERE profile_id=?`, profileID).Scan(&s.ExternalPapers)
	if err != nil {
		return nil, err
	}

	// papers_processed counts actual fetch attempts (last_citation_fetch_at
	// set), not "has an s2_paper_id" — a paper without DOI/arXiv/S2 id is
	// still "processed" (attempted and skipped), it just never gets an id.
	err = d.QueryRow(`SELECT COUNT(*) FROM papers WHERE profile_id=? AND last_citation_fetch_at IS NOT NULL`, profileID).Scan(&s.PapersProcessed)
	if err != nil {
		return nil, err
	}

	err = d.QueryRow(`SELECT COUNT(*) FROM papers WHERE profile_id=? AND status IN ('approved','downloaded')`, profileID).Scan(&s.PapersEligible)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// GetPapersForCitationGraph returns minimal paper data needed for graph nodes.
func (d *DB) GetPapersForCitationGraph(profileID int64, paperIDs []int64) ([]Paper, error) {
	if len(paperIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(paperIDs))
	args := make([]interface{}, len(paperIDs)+1)
	args[0] = profileID
	for i, id := range paperIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf(`SELECT id, profile_id, title, year, authors, doi, citation_count, status, s2_paper_id
		FROM papers WHERE profile_id=? AND id IN (%s)`, joinStrings(placeholders, ","))

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var papers []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.ID, &p.ProfileID, &p.Title, &p.Year, &p.Authors, &p.DOI,
			&p.CitationCount, &p.Status, &p.S2PaperID); err != nil {
			return nil, err
		}
		papers = append(papers, p)
	}
	return papers, rows.Err()
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// GetApprovedAndDownloadedPapers returns all papers with status approved or downloaded.
func (d *DB) GetApprovedAndDownloadedPapers(profileID int64) ([]Paper, error) {
	rows, err := d.Query(`SELECT id, profile_id, title, title_normalized, year, authors, doi, arxiv_id, citation_count, status, s2_paper_id, last_citation_fetch_at
		FROM papers WHERE profile_id=? AND status IN ('approved','downloaded')`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var papers []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.ID, &p.ProfileID, &p.Title, &p.TitleNormalized, &p.Year, &p.Authors,
			&p.DOI, &p.ArxivID, &p.CitationCount, &p.Status, &p.S2PaperID, &p.LastCitationFetchAt); err != nil {
			return nil, err
		}
		papers = append(papers, p)
	}
	return papers, rows.Err()
}
