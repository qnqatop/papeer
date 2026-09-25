package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/qnqatop/papeer/internal/ptr"
)

// UpsertPaper inserts a paper or merges it with an existing one (by DOI or normalized title).
// On conflict: merges sources, keeps best pdf_url, takes max citation_count, longest abstract.
func (d *DB) UpsertPaper(p *Paper) error {
	// Try to find existing paper by DOI or normalized title.
	var existingID int64
	var found bool

	if p.DOI != nil && *p.DOI != "" {
		err := d.QueryRow(`SELECT id FROM papers WHERE profile_id=? AND doi=?`, p.ProfileID, *p.DOI).Scan(&existingID)
		if err == nil {
			found = true
		}
	}
	if !found {
		err := d.QueryRow(`SELECT id FROM papers WHERE profile_id=? AND title_normalized=?`, p.ProfileID, p.TitleNormalized).Scan(&existingID)
		if err == nil {
			found = true
		}
	}

	if !found {
		if err := d.insertPaper(p); err != nil {
			return err
		}
		// Link to axis.
		if p.AxisID != nil {
			return d.LinkPaperAxis(p.ID, *p.AxisID)
		}
		return nil
	}

	if err := d.mergePaper(existingID, p); err != nil {
		return err
	}
	// Report the id of the row the paper was merged into, so callers can
	// load or update it (status, s2 id, ...).
	p.ID = existingID
	// Link existing paper to new axis.
	if p.AxisID != nil {
		return d.LinkPaperAxis(existingID, *p.AxisID)
	}
	return nil
}

func (d *DB) insertPaper(p *Paper) error {
	res, err := d.Exec(`INSERT INTO papers (profile_id, axis_id, title, title_normalized, abstract, year, venue, authors, doi, arxiv_id, pdf_url, pdf_source, citation_count, pre_score, score_reasons, sources, status, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ProfileID, p.AxisID, p.Title, p.TitleNormalized, p.Abstract, p.Year, p.Venue,
		p.Authors, p.DOI, p.ArxivID, p.PdfURL, p.PdfSource,
		p.CitationCount, p.PreScore, p.ScoreReasons, p.Sources, p.Status, p.Notes)
	if err != nil {
		return fmt.Errorf("insert paper: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (d *DB) mergePaper(existingID int64, incoming *Paper) error {
	// Load existing.
	existing, err := d.GetPaper(existingID)
	if err != nil {
		return err
	}

	// Merge sources.
	sourceSet := make(map[string]bool)
	for _, s := range existing.Sources {
		sourceSet[s] = true
	}
	for _, s := range incoming.Sources {
		sourceSet[s] = true
	}
	merged := make(JSONStringSlice, 0, len(sourceSet))
	for s := range sourceSet {
		merged = append(merged, s)
	}

	// Max citation count.
	cc := existing.CitationCount
	if incoming.CitationCount > cc {
		cc = incoming.CitationCount
	}

	// Longest abstract.
	abstract := existing.Abstract
	if len(incoming.Abstract) > len(abstract) {
		abstract = incoming.Abstract
	}

	// Best pdf_url by source priority.
	pdfURL := existing.PdfURL
	pdfSource := existing.PdfSource
	if incoming.PdfURL != nil {
		if pdfURL == nil || sourcePriority(ptr.Val(incoming.PdfSource)) < sourcePriority(ptr.Val(pdfSource)) {
			pdfURL = incoming.PdfURL
			pdfSource = incoming.PdfSource
		}
	}

	// Fill missing fields.
	doi := existing.DOI
	if doi == nil && incoming.DOI != nil {
		doi = incoming.DOI
	}
	arxivID := existing.ArxivID
	if arxivID == nil && incoming.ArxivID != nil {
		arxivID = incoming.ArxivID
	}
	venue := existing.Venue
	if venue == "" && incoming.Venue != "" {
		venue = incoming.Venue
	}

	// Higher score wins.
	score := existing.PreScore
	reasons := existing.ScoreReasons
	if incoming.PreScore > score {
		score = incoming.PreScore
		reasons = incoming.ScoreReasons
	}

	_, err = d.Exec(`UPDATE papers SET sources=?, citation_count=?, abstract=?, pdf_url=?, pdf_source=?, doi=?, arxiv_id=?, venue=?, pre_score=?, score_reasons=? WHERE id=?`,
		merged, cc, abstract, pdfURL, pdfSource, doi, arxivID, venue, score, reasons, existingID)
	return err
}

func sourcePriority(src string) int {
	priorities := map[string]int{
		"arxiv": 0, "semantic_scholar": 1, "openalex": 2,
		"unpaywall": 3, "crossref": 4, "s2_title": 5,
		"cyberleninka": 6,
	}
	if p, ok := priorities[src]; ok {
		return p
	}
	return 99
}

func (d *DB) GetPaper(id int64) (*Paper, error) {
	var p Paper
	err := d.QueryRow(`SELECT id, profile_id, axis_id, title, title_normalized, abstract, year, venue, authors, doi, arxiv_id, pdf_url, pdf_source, citation_count, pre_score, score_reasons, sources, status, user_score, notes, found_at, ai_match_score FROM papers WHERE id=?`, id).
		Scan(&p.ID, &p.ProfileID, &p.AxisID, &p.Title, &p.TitleNormalized, &p.Abstract, &p.Year, &p.Venue, &p.Authors, &p.DOI, &p.ArxivID, &p.PdfURL, &p.PdfSource, &p.CitationCount, &p.PreScore, &p.ScoreReasons, &p.Sources, &p.Status, &p.UserScore, &p.Notes, &p.FoundAt, &p.AiMatchScore)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// maxListLimit caps PaperFilter.Limit so a bogus value from the frontend
// can't load an unbounded result set.
const maxListLimit = 10000

// likeEscaper escapes LIKE metacharacters for use with ESCAPE '\'.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (d *DB) ListPapers(f PaperFilter) ([]Paper, int, error) {
	where := []string{"profile_id=?"}
	args := []interface{}{f.ProfileID}

	if f.AxisID != nil {
		where = append(where, "axis_id=?")
		args = append(args, *f.AxisID)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.MinScore > 0 {
		where = append(where, "pre_score>=?")
		args = append(args, f.MinScore)
	}
	if f.MaxScore > 0 {
		where = append(where, "pre_score<=?")
		args = append(args, f.MaxScore)
	}
	if f.MinCitations > 0 {
		where = append(where, "citation_count>=?")
		args = append(args, f.MinCitations)
	}
	if f.YearFrom > 0 {
		where = append(where, "year>=?")
		args = append(args, f.YearFrom)
	}
	if f.YearTo > 0 {
		where = append(where, "year<=?")
		args = append(args, f.YearTo)
	}
	if f.MinUserScore > 0 {
		where = append(where, "user_score>=?")
		args = append(args, f.MinUserScore)
	}
	if f.Search != "" {
		// Escape LIKE wildcards so "%" and "_" in user input match literally.
		where = append(where, `(title LIKE ? ESCAPE '\' OR abstract LIKE ? ESCAPE '\')`)
		like := "%" + likeEscaper.Replace(f.Search) + "%"
		args = append(args, like, like)
	}
	if f.HasSummary {
		where = append(where, "EXISTS (SELECT 1 FROM summaries WHERE summaries.paper_id = papers.id AND summaries.status = 'done')")
	}
	if len(f.TagIDs) > 0 {
		tagClause, tagArgs := buildTagFilter(f.TagIDs)
		where = append(where, tagClause)
		args = append(args, tagArgs...)
	}

	whereClause := strings.Join(where, " AND ")

	// Count total.
	var total int
	if err := d.QueryRow("SELECT COUNT(*) FROM papers WHERE "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Order.
	orderBy := "pre_score DESC, citation_count DESC"
	switch f.SortBy {
	case "year":
		orderBy = "year DESC NULLS LAST, pre_score DESC"
	case "citations":
		orderBy = "citation_count DESC, pre_score DESC"
	case "title":
		orderBy = "title ASC"
	case "ai_match_score":
		orderBy = "ai_match_score DESC"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	query := fmt.Sprintf(""+
		"SELECT id, profile_id, axis_id, title, title_normalized, abstract, year, venue, authors, doi, arxiv_id, pdf_url, pdf_source, citation_count, pre_score, score_reasons, sources, status, user_score, notes, found_at, ai_match_score FROM papers WHERE %s ORDER BY %s LIMIT ? OFFSET ?", whereClause, orderBy)
	args = append(args, limit, f.Offset)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.ID, &p.ProfileID, &p.AxisID, &p.Title, &p.TitleNormalized, &p.Abstract, &p.Year, &p.Venue, &p.Authors, &p.DOI, &p.ArxivID, &p.PdfURL, &p.PdfSource, &p.CitationCount, &p.PreScore, &p.ScoreReasons, &p.Sources, &p.Status, &p.UserScore, &p.Notes, &p.FoundAt, &p.AiMatchScore); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Enrich with axis_count and tags.
	if err := d.enrichPapers(out); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (d *DB) UpdatePaperStatus(id int64, status string) error {
	_, err := d.Exec(`UPDATE papers SET status=? WHERE id=?`, status, id)
	return err
}

func (d *DB) BulkUpdateStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, status)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	res, err := d.Exec(fmt.Sprintf("UPDATE papers SET status=? WHERE id IN (%s)", strings.Join(placeholders, ",")), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) SetUserScore(id int64, score int) error {
	_, err := d.Exec(`UPDATE papers SET user_score=? WHERE id=?`, score, id)
	return err
}

func (d *DB) SetPaperNotes(id int64, notes string) error {
	_, err := d.Exec(`UPDATE papers SET notes=? WHERE id=?`, notes, id)
	return err
}

// ApproveByScore approves all papers with pre_score >= minScore for a profile.
func (d *DB) ApproveByScore(profileID int64, minScore int) (int64, error) {
	res, err := d.Exec(`UPDATE papers SET status='approved' WHERE profile_id=? AND status='new' AND pre_score>=?`, profileID, minScore)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AxesWithPapers returns axes that have at least one paper for the given profile.
func (d *DB) AxesWithPapers(profileID int64) ([]Axis, error) {
	rows, err := d.Query(`SELECT DISTINCT a.id, a.axis_key FROM axes a INNER JOIN papers p ON p.axis_id = a.id WHERE a.profile_id=? ORDER BY a.position`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Axis
	for rows.Next() {
		var a Axis
		if err := rows.Scan(&a.ID, &a.AxisKey); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// statsWhere builds a FROM+WHERE clause for stats queries with an optional
// axis filter. The axis filter goes through the paper_axes M2M table (a
// paper can belong to several axes), not the legacy papers.axis_id column,
// so counts reflect all axes a paper was actually found in.
func statsWhere(profileID int64, axisID *int64) (from string, where string, args []interface{}) {
	if axisID != nil {
		return "papers INNER JOIN paper_axes ON paper_axes.paper_id = papers.id",
			"papers.profile_id=? AND paper_axes.axis_id=?",
			[]interface{}{profileID, *axisID}
	}
	return "papers", "papers.profile_id=?", []interface{}{profileID}
}

// PapersByStatus returns counts grouped by status for a profile.
func (d *DB) PapersByStatus(profileID int64, axisID *int64) (map[string]int, error) {
	from, where, args := statsWhere(profileID, axisID)
	rows, err := d.Query(`SELECT status, COUNT(*) FROM `+from+` WHERE `+where+` GROUP BY status`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		out[status] = count
	}
	return out, rows.Err()
}

// ScoreDistribution returns paper counts by pre_score.
func (d *DB) ScoreDistribution(profileID int64, axisID *int64) (map[int]int, error) {
	from, where, args := statsWhere(profileID, axisID)
	rows, err := d.Query(`SELECT pre_score, COUNT(*) FROM `+from+` WHERE `+where+` GROUP BY pre_score ORDER BY pre_score`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]int)
	for rows.Next() {
		var score, count int
		rows.Scan(&score, &count)
		out[score] = count
	}
	return out, rows.Err()
}

// YearDistribution returns paper counts by year.
func (d *DB) YearDistribution(profileID int64, axisID *int64) (map[int]int, error) {
	from, where, args := statsWhere(profileID, axisID)
	rows, err := d.Query(`SELECT year, COUNT(*) FROM `+from+` WHERE `+where+` AND year IS NOT NULL GROUP BY year ORDER BY year`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]int)
	for rows.Next() {
		var year sql.NullInt64
		var count int
		rows.Scan(&year, &count)
		if year.Valid {
			out[int(year.Int64)] = count
		}
	}
	return out, rows.Err()
}

// TopAuthors returns the most frequent authors across a profile's papers
// (optionally scoped to one axis via the paper_axes M2M table), sorted
// descending by paper count. Authors are read from the JSON authors column
// in Go since SQLite has no native JSON array aggregation we can rely on
// across all supported versions.
func (d *DB) TopAuthors(profileID int64, axisID *int64, limit int) ([]AuthorCount, error) {
	from, where, args := statsWhere(profileID, axisID)
	rows, err := d.Query(`SELECT papers.authors FROM `+from+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var authors JSONStringSlice
		if err := rows.Scan(&authors); err != nil {
			return nil, err
		}
		for _, name := range authors {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			counts[name]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]AuthorCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, AuthorCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// citationBucketBounds defines the citation-count histogram buckets, in order.
var citationBucketBounds = []struct {
	label    string
	min, max int // max = -1 means unbounded
}{
	{"0", 0, 0},
	{"1-5", 1, 5},
	{"6-20", 6, 20},
	{"21-50", 21, 50},
	{"51-100", 51, 100},
	{"100+", 101, -1},
}

// CitationCountBuckets returns a histogram of citation_count for a profile
// (optionally scoped to an axis), in a stable bucket order suitable for
// direct chart rendering.
func (d *DB) CitationCountBuckets(profileID int64, axisID *int64) ([]CitationBucket, error) {
	from, where, args := statsWhere(profileID, axisID)
	rows, err := d.Query(`SELECT papers.citation_count FROM `+from+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := make([]CitationBucket, len(citationBucketBounds))
	for i, b := range citationBucketBounds {
		buckets[i] = CitationBucket{Label: b.label}
	}

	for rows.Next() {
		var cc int
		if err := rows.Scan(&cc); err != nil {
			return nil, err
		}
		for i, b := range citationBucketBounds {
			if cc >= b.min && (b.max == -1 || cc <= b.max) {
				buckets[i].Count++
				break
			}
		}
	}
	return buckets, rows.Err()
}

// PDFAvailability describes what fraction of approved/downloaded papers have
// a successfully downloaded PDF on disk.
type PDFAvailability struct {
	Approved int     `json:"approved"`
	WithPDF  int     `json:"with_pdf"`
	Percent  float64 `json:"percent"`
}

// GetPDFAvailability computes the share of approved/downloaded papers that
// have at least one successful download recorded.
func (d *DB) GetPDFAvailability(profileID int64) (*PDFAvailability, error) {
	var out PDFAvailability
	err := d.QueryRow(`SELECT COUNT(*) FROM papers WHERE profile_id=? AND status IN ('approved','downloaded')`, profileID).
		Scan(&out.Approved)
	if err != nil {
		return nil, err
	}
	err = d.QueryRow(`SELECT COUNT(DISTINCT p.id) FROM papers p
		JOIN downloads d ON d.paper_id = p.id
		WHERE p.profile_id=? AND p.status IN ('approved','downloaded') AND d.status='ok'`, profileID).
		Scan(&out.WithPDF)
	if err != nil {
		return nil, err
	}
	if out.Approved > 0 {
		out.Percent = float64(out.WithPDF) / float64(out.Approved) * 100
	}
	return &out, nil
}

// GetApprovedPapers returns papers with status "approved" for downloading.
func (d *DB) GetApprovedPapers(profileID int64) ([]Paper, error) {
	papers, _, err := d.ListPapers(PaperFilter{
		ProfileID: profileID,
		Status:    "approved",
		Limit:     10000,
	})
	return papers, err
}

// ExportPapers returns all papers (or filtered) for export.
func (d *DB) ExportPapers(profileID int64) ([]Paper, error) {
	papers, _, err := d.ListPapers(PaperFilter{
		ProfileID: profileID,
		Limit:     100000,
	})
	return papers, err
}

// LinkPaperAxis records that a paper was found via a specific axis.
func (d *DB) LinkPaperAxis(paperID, axisID int64) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO paper_axes (paper_id, axis_id) VALUES (?, ?)`, paperID, axisID)
	return err
}

// PaperAxisCount returns the number of axes each paper was found in.
// Returns a map of paperID → count.
func (d *DB) PaperAxisCounts(paperIDs []int64) (map[int64]int, error) {
	if len(paperIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(paperIDs))
	args := make([]interface{}, len(paperIDs))
	for i, id := range paperIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := d.Query(
		fmt.Sprintf(`SELECT paper_id, COUNT(*) FROM paper_axes WHERE paper_id IN (%s) GROUP BY paper_id`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]int)
	for rows.Next() {
		var pid int64
		var cnt int
		if err := rows.Scan(&pid, &cnt); err != nil {
			return nil, err
		}
		out[pid] = cnt
	}
	return out, rows.Err()
}

// PaperTagsMulti returns tags for multiple papers at once.
func (d *DB) PaperTagsMulti(paperIDs []int64) (map[int64][]PaperTag, error) {
	if len(paperIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(paperIDs))
	args := make([]interface{}, len(paperIDs))
	for i, id := range paperIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := d.Query(
		fmt.Sprintf(`SELECT pt.paper_id, t.id, t.name, t.color FROM paper_tags pt INNER JOIN tags t ON t.id=pt.tag_id WHERE pt.paper_id IN (%s) ORDER BY t.name`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64][]PaperTag)
	for rows.Next() {
		var pid int64
		var pt PaperTag
		if err := rows.Scan(&pid, &pt.TagID, &pt.Name, &pt.Color); err != nil {
			return nil, err
		}
		out[pid] = append(out[pid], pt)
	}
	return out, rows.Err()
}

// enrichPapers fills AxisCount and Tags for a slice of papers.
func (d *DB) enrichPapers(papers []Paper) error {
	if len(papers) == 0 {
		return nil
	}
	ids := make([]int64, len(papers))
	for i, p := range papers {
		ids[i] = p.ID
	}

	axisCounts, err := d.PaperAxisCounts(ids)
	if err != nil {
		return err
	}
	tagMap, err := d.PaperTagsMulti(ids)
	if err != nil {
		return err
	}

	for i := range papers {
		pid := papers[i].ID
		papers[i].AxisCount = axisCounts[pid]
		if tags, ok := tagMap[pid]; ok {
			papers[i].Tags = tags
		}
	}
	return nil
}

// UpdateAIScores массово обновляет AI-скор для статей
func (d *DB) UpdateAIScores(scores map[int64]int) error {
	if len(scores) == 0 {
		return nil
	}

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE papers SET ai_match_score = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id, score := range scores {
		if _, err := stmt.Exec(score, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetDownloadedPapers returns papers with status='downloaded' that have a
// successful download entry (downloads.status='ok' and filename IS NOT NULL).
func (d *DB) GetDownloadedPapers(profileID int64) ([]Paper, error) {
	rows, err := d.Query(`SELECT DISTINCT p.id, p.profile_id, p.axis_id, p.title, p.title_normalized, p.abstract, p.year, p.venue, p.authors, p.doi, p.arxiv_id, p.pdf_url, p.pdf_source, p.citation_count, p.pre_score, p.score_reasons, p.sources, p.status, p.user_score, p.notes, p.found_at, p.ai_match_score
		FROM papers p
		JOIN downloads d ON d.paper_id = p.id
		WHERE p.profile_id=? AND p.status='downloaded' AND d.status='ok' AND d.filename IS NOT NULL`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.ID, &p.ProfileID, &p.AxisID, &p.Title, &p.TitleNormalized, &p.Abstract, &p.Year, &p.Venue, &p.Authors, &p.DOI, &p.ArxivID, &p.PdfURL, &p.PdfSource, &p.CitationCount, &p.PreScore, &p.ScoreReasons, &p.Sources, &p.Status, &p.UserScore, &p.Notes, &p.FoundAt, &p.AiMatchScore); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
