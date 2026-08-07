package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"
	"github.com/qnqatop/papeer/internal/search"
)

// ── Citation Graph Types ────────────────────────────────

// GraphNode represents a node in the citation graph.
type GraphNode struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Title         string   `json:"title"`
	Year          *int     `json:"year"`
	CitationCount int      `json:"citation_count"`
	Authors       []string `json:"authors"`
	Status        string   `json:"status"`
	NodeType      string   `json:"node_type"` // "internal" or "external"
	PaperID       *int64   `json:"paper_id"`
	MentionCount  int      `json:"mention_count"`
}

// GraphEdge represents an edge in the citation graph.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// CitationGraphData is the full graph payload for the frontend.
type CitationGraphData struct {
	Nodes []GraphNode      `json:"nodes"`
	Edges []GraphEdge      `json:"edges"`
	Stats db.CitationStats `json:"stats"`
}

// CitationProgress is emitted as the citation:progress / citation:done event.
type CitationProgress struct {
	Type    string `json:"type"` // "paper", "done", "error"
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Title   string `json:"title"`
	// Message is a human-readable summary, kept for backwards compatibility.
	// The frontend should prefer the structural fields below instead of
	// parsing this string.
	Message string `json:"message"`
	// Reason is a UI-friendly explanation set on "done" when the fetch hit a
	// provider block (e.g. S2 returned 403 for every paper). Empty otherwise.
	Reason string `json:"reason,omitempty"`
	// Errors counts papers that could not be resolved/fetched (no DOI/arXiv
	// id, or an S2 lookup failure); non-zero with empty results means the
	// graph is empty because of upstream errors, not lack of citations.
	Errors int `json:"errors,omitempty"`

	// Structural stats for the terminal "done" event. Prefer these over
	// parsing Message.
	InternalLinks   int `json:"internal_links,omitempty"`
	ExternalPapers  int `json:"external_papers,omitempty"`
	PapersProcessed int `json:"papers_processed,omitempty"`
	PapersEligible  int `json:"papers_eligible,omitempty"`
}

var citationMu sync.Mutex

// ── Citation Graph Methods ──────────────────────────────

// FetchCitations fetches citation data for all approved/downloaded papers.
// It resumes automatically: papers already processed by a previous run
// (papers.last_citation_fetch_at set) are skipped, so a cancelled or
// interrupted fetch can simply be re-run instead of starting over. Call
// ClearCitationData first to force a full re-fetch from scratch.
func (a *App) FetchCitations(profileID int64) error {
	if !citationMu.TryLock() {
		return fmt.Errorf("citation fetch already in progress")
	}

	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		citationMu.Unlock()
		return err
	}
	if !IsValidEmail(profile.Email) {
		citationMu.Unlock()
		return ErrInvalidEmail
	}

	papers, err := a.db.GetApprovedAndDownloadedPapers(profileID)
	if err != nil {
		citationMu.Unlock()
		return err
	}

	if len(papers) == 0 {
		citationMu.Unlock()
		return fmt.Errorf("no approved or downloaded papers")
	}

	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		citationMu.Unlock()
		return err
	}

	fetcher := search.NewCitationFetcher(client)

	ctx, cancel := context.WithCancel(a.ctx)
	a.setCancel(cancel)

	go func() {
		defer citationMu.Unlock()
		defer func() {
			cancel()
			a.clearCancel()
		}()

		a.fetchCitationsWorker(ctx, profileID, papers, fetcher)
	}()

	return nil
}

// citationJob pairs a paper with its resolved S2 lookup key, ready to be
// sent to the batch endpoint.
type citationJob struct {
	paper db.Paper
	key   string
}

func (a *App) fetchCitationsWorker(ctx context.Context, profileID int64, papers []db.Paper, fetcher *search.CitationFetcher) {
	total := len(papers)

	// Build lookup maps from the FULL eligible set (not just pending papers)
	// so references/citations can match against papers that were already
	// resolved in an earlier run.
	titleMap := make(map[string]int64, len(papers))
	doiMap := make(map[string]int64, len(papers))
	for _, p := range papers {
		titleMap[p.TitleNormalized] = p.ID
		if p.DOI != nil && *p.DOI != "" {
			doiMap[strings.ToLower(*p.DOI)] = p.ID
		}
	}

	// Resume: skip papers already attempted by a previous run.
	processed := 0
	var jobs []citationJob
	var errCount int
	for _, p := range papers {
		if p.LastCitationFetchAt != nil {
			processed++
			continue
		}
		key := search.ResolvePaperID(p.DOI, p.ArxivID, p.S2PaperID)
		if key == "" {
			// Nothing to look up (no DOI/arXiv/S2 id) — mark as attempted so
			// we don't retry it forever, and count it as an error so the UI
			// can explain an empty result.
			errCount++
			processed++
			if err := a.db.SetLastCitationFetchAt(p.ID, time.Now()); err != nil {
				runtime.LogWarningf(a.ctx, "citation: mark processed %d: %v", p.ID, err)
			}
			continue
		}
		jobs = append(jobs, citationJob{paper: p, key: key})
	}

	if len(jobs) == 0 {
		a.emitCitationDone(profileID, processed, total, errCount, "")
		return
	}

	var blockReason string
	const chunkSize = search.BatchSize
	for start := 0; start < len(jobs); start += chunkSize {
		if ctx.Err() != nil {
			return
		}
		end := min(start+chunkSize, len(jobs))
		chunk := jobs[start:end]

		keys := make([]string, len(chunk))
		for i, j := range chunk {
			keys[i] = j.key
		}

		runtime.EventsEmit(a.ctx, "citation:progress", CitationProgress{
			Type:    "paper",
			Current: processed,
			Total:   total,
			Title:   fmt.Sprintf("Fetching batch %d–%d of %d…", start+1, end, len(jobs)),
		})

		results, err := fetcher.FetchBatch(ctx, keys)
		if err != nil {
			runtime.LogWarningf(a.ctx, "citation: batch fetch (%d papers): %v", len(chunk), err)
			errCount += len(chunk) - len(results)
			if r := search.ProviderBlockReason(err); r != "" {
				blockReason = r
				// Circuit breaker: stop entirely instead of hammering S2 with
				// further batches. Unattempted papers keep last_citation_fetch_at
				// unset so the next run (resume) retries them.
				remaining := len(jobs) - end
				errCount += remaining
				break
			}
			// Non-blocking error (e.g. transient network issue): leave these
			// papers unmarked so resume retries them, and move on.
			continue
		}

		for i, p := range chunk {
			if ctx.Err() != nil {
				return
			}
			processed++
			runtime.EventsEmit(a.ctx, "citation:progress", CitationProgress{
				Type:    "paper",
				Current: processed,
				Total:   total,
				Title:   truncateTitle(p.paper.Title, 60),
			})

			var res *search.S2BatchPaper
			if i < len(results) {
				res = results[i]
			}
			if res == nil {
				errCount++
				if err := a.db.SetLastCitationFetchAt(p.paper.ID, time.Now()); err != nil {
					runtime.LogWarningf(a.ctx, "citation: mark processed %d: %v", p.paper.ID, err)
				}
				continue
			}

			if res.S2PaperID != "" && (p.paper.S2PaperID == nil || *p.paper.S2PaperID == "") {
				if err := a.db.SetPaperS2ID(p.paper.ID, res.S2PaperID); err != nil {
					runtime.LogWarningf(a.ctx, "citation: set s2 id for paper %d: %v", p.paper.ID, err)
				}
			}

			a.processCitationEntries(profileID, p.paper.ID, res.References, titleMap, doiMap, true)
			a.processCitationEntries(profileID, p.paper.ID, res.Citations, titleMap, doiMap, false)

			if err := a.db.SetLastCitationFetchAt(p.paper.ID, time.Now()); err != nil {
				runtime.LogWarningf(a.ctx, "citation: mark processed %d: %v", p.paper.ID, err)
			}
		}
	}

	a.emitCitationDone(profileID, processed, total, errCount, blockReason)
}

// emitCitationDone loads final graph stats and emits the terminal
// citation:done event with both a human-readable summary and structural
// fields for the frontend.
func (a *App) emitCitationDone(profileID int64, current, total, errCount int, blockReason string) {
	stats, err := a.db.GetCitationStats(profileID)
	progress := CitationProgress{
		Type:    "done",
		Current: current,
		Total:   total,
		Reason:  blockReason,
		Errors:  errCount,
	}
	if err == nil && stats != nil {
		progress.Message = fmt.Sprintf("%d internal links, %d external papers", stats.InternalLinks, stats.ExternalPapers)
		progress.InternalLinks = stats.InternalLinks
		progress.ExternalPapers = stats.ExternalPapers
		progress.PapersProcessed = stats.PapersProcessed
		progress.PapersEligible = stats.PapersEligible
	}
	runtime.EventsEmit(a.ctx, "citation:done", progress)
}

func (a *App) processCitationEntries(profileID, sourcePaperID int64, entries []search.S2CitationEntry,
	titleMap map[string]int64, doiMap map[string]int64, isReference bool) {

	for _, entry := range entries {
		// Try to match to a known paper.
		var matchedID int64

		// Match by DOI.
		if doi := strings.ToLower(entry.ExternalIDs.DOI); doi != "" {
			if id, ok := doiMap[doi]; ok {
				matchedID = id
			}
		}

		// Match by normalized title.
		if matchedID == 0 {
			normTitle := search.NormalizeTitle(entry.Title)
			if id, ok := titleMap[normTitle]; ok {
				matchedID = id
			}
		}

		if matchedID != 0 && matchedID != sourcePaperID {
			// Internal link: both papers are in the user's database.
			if isReference {
				// sourcePaper references matchedPaper
				if err := a.db.UpsertCitationLink(profileID, sourcePaperID, matchedID); err != nil {
					runtime.LogWarningf(a.ctx, "citation: upsert link %d→%d: %v", sourcePaperID, matchedID, err)
				}
			} else {
				// matchedPaper references sourcePaper (matchedPaper cites source)
				if err := a.db.UpsertCitationLink(profileID, matchedID, sourcePaperID); err != nil {
					runtime.LogWarningf(a.ctx, "citation: upsert link %d→%d: %v", matchedID, sourcePaperID, err)
				}
			}
		} else if matchedID == 0 {
			// External citation: not in user's database. Record both the
			// citation itself and which internal paper mentioned it, so
			// Coverage Gaps can show "who cites this".
			authors := search.ExtractAuthors(entry.Authors)
			extID, err := a.db.UpsertExternalCitationGetID(profileID, &db.ExternalCitation{
				S2PaperID:     entry.S2PaperID,
				Title:         entry.Title,
				Year:          entry.Year,
				CitationCount: search.CitationCountVal(entry),
				Authors:       authors,
			})
			if err != nil {
				runtime.LogWarningf(a.ctx, "citation: upsert external %q: %v", entry.Title, err)
				continue
			}
			if err := a.db.UpsertCitationMention(sourcePaperID, extID); err != nil {
				runtime.LogWarningf(a.ctx, "citation: upsert mention (paper=%d, external=%d): %v", sourcePaperID, extID, err)
			}
		}
	}
}

// GetCitationGraph builds and returns the citation graph data.
func (a *App) GetCitationGraph(profileID int64, minMentions int) (*CitationGraphData, error) {
	links, err := a.db.GetCitationLinks(profileID)
	if err != nil {
		return nil, err
	}

	externals, err := a.db.GetExternalCitations(profileID, minMentions, 50)
	if err != nil {
		return nil, err
	}

	stats, err := a.db.GetCitationStats(profileID)
	if err != nil {
		return nil, err
	}

	// Collect all paper IDs that appear in links.
	paperIDSet := make(map[int64]bool)
	for _, l := range links {
		paperIDSet[l.FromPaperID] = true
		paperIDSet[l.ToPaperID] = true
	}

	paperIDs := make([]int64, 0, len(paperIDSet))
	for id := range paperIDSet {
		paperIDs = append(paperIDs, id)
	}

	papers, err := a.db.GetPapersForCitationGraph(profileID, paperIDs)
	if err != nil {
		return nil, err
	}

	// Build nodes.
	nodes := make([]GraphNode, 0, len(papers)+len(externals))
	for _, p := range papers {
		var authors []string
		if len(p.Authors) > 0 {
			authors = []string(p.Authors)
		}
		nodes = append(nodes, GraphNode{
			ID:            fmt.Sprintf("p_%d", p.ID),
			Label:         truncateTitle(p.Title, 40),
			Title:         p.Title,
			Year:          p.Year,
			CitationCount: p.CitationCount,
			Authors:       authors,
			Status:        p.Status,
			NodeType:      "internal",
			PaperID:       &p.ID,
		})
	}

	for _, ec := range externals {
		var authors []string
		if len(ec.Authors) > 0 {
			authors = []string(ec.Authors)
		}
		nodes = append(nodes, GraphNode{
			ID:            fmt.Sprintf("ext_%d", ec.ID),
			Label:         truncateTitle(ec.Title, 40),
			Title:         ec.Title,
			Year:          ec.Year,
			CitationCount: ec.CitationCount,
			Authors:       authors,
			Status:        "external",
			NodeType:      "external",
			MentionCount:  ec.MentionCount,
		})
	}

	// Build edges.
	edges := make([]GraphEdge, 0, len(links))
	for _, l := range links {
		edges = append(edges, GraphEdge{
			Source: fmt.Sprintf("p_%d", l.FromPaperID),
			Target: fmt.Sprintf("p_%d", l.ToPaperID),
		})
	}

	return &CitationGraphData{
		Nodes: nodes,
		Edges: edges,
		Stats: *stats,
	}, nil
}

// GetMissingKeyPapers returns external papers most referenced by user's papers.
func (a *App) GetMissingKeyPapers(profileID int64, limit int) ([]db.ExternalCitation, error) {
	if limit <= 0 {
		limit = 20
	}
	return a.db.GetExternalCitations(profileID, 2, limit)
}

// GetCoverageGaps returns external citations (candidate "missing" papers)
// with mention_count >= minMentions, sorted by mention_count desc, each
// annotated with the internal paper IDs that reference or are cited
// alongside it. This is the data backing the "Coverage Gaps" tab.
func (a *App) GetCoverageGaps(profileID int64, minMentions, limit int) ([]db.ExternalCitationWithMentions, error) {
	if minMentions <= 0 {
		minMentions = 2
	}
	if limit <= 0 {
		limit = 50
	}
	return a.db.GetCoverageGaps(profileID, minMentions, limit)
}

// ResolveAndAddExternal promotes a Coverage Gaps external citation to a full
// paper in the profile's library: it fetches full metadata from Semantic
// Scholar (abstract, DOI, venue, open-access PDF — fields external_citations
// doesn't store) and inserts it with the given status.
//
// axisID, if non-nil, links the new paper to that axis. status defaults to
// "approved" when empty. The "Add & Download" UI action is expected to call
// this with status "approved" and then invoke the existing DownloadApproved
// method — no separate download path is needed here.
func (a *App) ResolveAndAddExternal(profileID, externalID int64, axisID *int64, status string) (*db.Paper, error) {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return nil, err
	}
	if !IsValidEmail(profile.Email) {
		return nil, ErrInvalidEmail
	}

	ec, err := a.db.GetExternalCitationByID(profileID, externalID)
	if err != nil {
		return nil, err
	}

	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		return nil, err
	}
	fetcher := search.NewCitationFetcher(client)

	raw, err := fetcher.FetchPaperDetail(a.ctx, ec.S2PaperID)
	if err != nil {
		// Fall back to whatever the external_citations row already has —
		// still better than failing the whole action outright.
		runtime.LogWarningf(a.ctx, "resolve external %d (%s): %v", externalID, ec.S2PaperID, err)
		raw = search.RawPaper{
			Title:         ec.Title,
			Year:          ec.Year,
			Authors:       []string(ec.Authors),
			CitationCount: ec.CitationCount,
			Source:        "semantic_scholar",
		}
	}
	if strings.TrimSpace(raw.Title) == "" {
		raw.Title = ec.Title
	}

	var keywords []db.Keyword
	if axisID != nil {
		if axis, err := a.db.GetAxis(*axisID); err == nil {
			keywords = axis.Keywords
		}
	}
	sr := search.ScorePaper(raw, keywords)

	if status == "" {
		status = "approved"
	}

	paper := &db.Paper{
		ProfileID:       profileID,
		AxisID:          axisID,
		Title:           raw.Title,
		TitleNormalized: search.NormalizeTitle(raw.Title),
		Abstract:        raw.Abstract,
		Year:            raw.Year,
		Venue:           raw.Venue,
		Authors:         db.JSONStringSlice(raw.Authors),
		DOI:             ptr.Ptr(raw.DOI),
		ArxivID:         ptr.Ptr(raw.ArxivID),
		PdfURL:          ptr.Ptr(raw.PdfURL),
		PdfSource:       ptr.Ptr(raw.Source),
		CitationCount:   raw.CitationCount,
		PreScore:        sr.Score,
		ScoreReasons:    sr.Reasons,
		Sources:         []string{raw.Source},
		Status:          status,
	}
	if err := a.db.UpsertPaper(paper); err != nil {
		return nil, fmt.Errorf("add external paper: %w", err)
	}
	if ec.S2PaperID != "" {
		if err := a.db.SetPaperS2ID(paper.ID, ec.S2PaperID); err != nil {
			runtime.LogWarningf(a.ctx, "set s2 id for new paper %d: %v", paper.ID, err)
		}
	}

	return a.db.GetPaper(paper.ID)
}

// ClearCitationData deletes all citation data for a profile and resets the
// per-paper fetch-progress marker (see db.ClearCitationData) so a subsequent
// FetchCitations starts fresh.
func (a *App) ClearCitationData(profileID int64) error {
	return a.db.ClearCitationData(profileID)
}

func truncateTitle(title string, maxLen int) string {
	runes := []rune(title)
	if len(runes) <= maxLen {
		return title
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
