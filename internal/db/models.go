package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSONStringSlice is a []string stored as JSON text in SQLite.
type JSONStringSlice []string

func (j *JSONStringSlice) Scan(src interface{}) error {
	if src == nil {
		*j = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return fmt.Errorf("JSONStringSlice.Scan: unsupported type %T", src)
	}
	return json.Unmarshal(b, j)
}

func (j JSONStringSlice) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Profile represents a research profile (search_profiles table).
type Profile struct {
	ID              int64           `json:"id"`
	Name            string          `json:"name"`
	Email           string          `json:"email"`
	PdfDir          string          `json:"pdf_dir"`
	YearMin         int             `json:"year_min"`
	VintageYear     int             `json:"vintage_year"`
	MaxPerQuery     int             `json:"max_per_query"`
	DownloadSources JSONStringSlice `json:"download_sources"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Axis represents a research axis (axes table).
type Axis struct {
	ID          int64  `json:"id"`
	ProfileID   int64  `json:"profile_id"`
	AxisKey     string `json:"axis_key"`
	Description string `json:"description"`
	YearMin     *int   `json:"year_min"`      // nil = inherit from profile
	MaxPerQuery *int   `json:"max_per_query"` // nil = inherit from profile
	Position    int    `json:"position"`

	// Loaded via joins, not stored directly in axes table.
	Queries  []Query   `json:"queries"`
	Keywords []Keyword `json:"keywords"`
}

// Query represents a search query (queries table).
type Query struct {
	ID       int64  `json:"id"`
	AxisID   int64  `json:"axis_id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
}

// Keyword represents a keyword filter (keywords table).
type Keyword struct {
	ID     int64  `json:"id"`
	AxisID int64  `json:"axis_id"`
	Word   string `json:"word"`
	Type   string `json:"type"` // "must" or "boost"
}

// Paper represents a found paper (papers table).
type Paper struct {
	ID              int64           `json:"id"`
	ProfileID       int64           `json:"profile_id"`
	AxisID          *int64          `json:"axis_id"`
	Title           string          `json:"title"`
	TitleNormalized string          `json:"title_normalized"`
	Abstract        string          `json:"abstract"`
	Year            *int            `json:"year"`
	Venue           string          `json:"venue"`
	Authors         JSONStringSlice `json:"authors"`
	DOI             *string         `json:"doi"`
	ArxivID         *string         `json:"arxiv_id"`
	PdfURL          *string         `json:"pdf_url"`
	PdfSource       *string         `json:"pdf_source"`
	CitationCount   int             `json:"citation_count"`
	PreScore        int             `json:"pre_score"`
	ScoreReasons    JSONStringSlice `json:"score_reasons"`
	Sources         JSONStringSlice `json:"sources"`
	Status          string          `json:"status"` // new, approved, rejected, downloaded
	UserScore       *int            `json:"user_score"`
	Notes           string          `json:"notes"`
	FoundAt         time.Time       `json:"found_at"`

	AiMatchScore int     `json:"ai_match_score"` // recsys
	S2PaperID    *string `json:"s2_paper_id"`    // Semantic Scholar paper ID

	// LastCitationFetchAt marks when the citation-fetch worker last attempted
	// this paper (resolved or not). nil = never attempted. Drives resume
	// (skip already-processed papers) and the papers_processed stat.
	LastCitationFetchAt *time.Time `json:"last_citation_fetch_at"`

	// Enriched fields (not stored in papers table directly).
	AxisCount int        `json:"axis_count"` // number of axes this paper was found in
	Tags      []PaperTag `json:"tags"`       // tags assigned to this paper
}

// Download represents a download attempt (downloads table).
type Download struct {
	ID          int64     `json:"id"`
	PaperID     int64     `json:"paper_id"`
	Source      string    `json:"source"`
	URL         *string   `json:"url"`
	Status      string    `json:"status"` // ok, fail, skip
	Reason      string    `json:"reason"`
	Filename    *string   `json:"filename"`
	FileSize    *int64    `json:"file_size"`
	AttemptedAt time.Time `json:"attempted_at"`
}

// PaperFilter defines filters for listing papers.
type PaperFilter struct {
	ProfileID    int64   `json:"profile_id"`
	AxisID       *int64  `json:"axis_id,omitempty"`
	Status       string  `json:"status,omitempty"`
	MinScore     int     `json:"min_score,omitempty"`
	MaxScore     int     `json:"max_score,omitempty"`
	MinCitations int     `json:"min_citations,omitempty"`
	YearFrom     int     `json:"year_from,omitempty"`
	YearTo       int     `json:"year_to,omitempty"`
	MinUserScore int     `json:"min_user_score,omitempty"`
	Search       string  `json:"search,omitempty"`
	TagIDs       []int64 `json:"tag_ids,omitempty"`
	HasSummary   bool    `json:"has_summary,omitempty"`
	SortBy       string  `json:"sort_by,omitempty"` // score, year, citations, title
	Limit        int     `json:"limit,omitempty"`
	Offset       int     `json:"offset,omitempty"`
}

// DownloadStats holds aggregated download statistics.
type DownloadStats struct {
	TotalAttempts int            `json:"total_attempts"`
	BySource      map[string]int `json:"by_source"`
	ByStatus      map[string]int `json:"by_status"`
}

// Tag represents a user-defined label for papers.
type Tag struct {
	ID        int64  `json:"id"`
	ProfileID int64  `json:"profile_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
}

// PaperTag is a lightweight struct for listing tags on a paper.
type PaperTag struct {
	TagID int64  `json:"tag_id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Summary represents an LLM-generated paper summary.
type Summary struct {
	ID         int64     `json:"id"`
	PaperID    int64     `json:"paper_id"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	PromptHash string    `json:"prompt_hash"`
	Content    string    `json:"content"`
	TokensIn   int       `json:"tokens_in"`
	TokensOut  int       `json:"tokens_out"`
	Status     string    `json:"status"`
	ErrorMsg   string    `json:"error_msg"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SummaryWithPaper joins summary with paper metadata for list views.
type SummaryWithPaper struct {
	Summary
	PaperTitle   string          `json:"paper_title"`
	PaperYear    *int            `json:"paper_year"`
	PaperAuthors JSONStringSlice `json:"paper_authors"`
	AxisID       *int64          `json:"axis_id"`
	AxisKey      string          `json:"axis_key"`
}

// LLMProfile represents an OpenAI-compatible LLM connection (llm_profiles table).
// The API key is not part of the model — it lives in the OS keychain.
type LLMProfile struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	BaseURL      string          `json:"base_url"`
	Models       JSONStringSlice `json:"models"`
	DefaultModel string          `json:"default_model"`
	Temperature  float64         `json:"temperature"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
}

// CitationLink represents a citation relationship between two papers in the database.
type CitationLink struct {
	ID          int64 `json:"id"`
	ProfileID   int64 `json:"profile_id"`
	FromPaperID int64 `json:"from_paper_id"`
	ToPaperID   int64 `json:"to_paper_id"`
}

// ExternalCitation represents a paper not in the user's database
// that is referenced by or cites known papers.
type ExternalCitation struct {
	ID            int64           `json:"id"`
	ProfileID     int64           `json:"profile_id"`
	S2PaperID     string          `json:"s2_paper_id"`
	Title         string          `json:"title"`
	Year          *int            `json:"year"`
	CitationCount int             `json:"citation_count"`
	Authors       JSONStringSlice `json:"authors"`
	MentionCount  int             `json:"mention_count"`
}

// ExternalCitationWithMentions extends ExternalCitation with the internal
// paper IDs that mention it (via a reference or a citing relationship). This
// is the data behind the "Coverage Gaps" tab: "what am I missing, and which
// of my papers pointed me at it".
type ExternalCitationWithMentions struct {
	ExternalCitation
	MentionedBy []int64 `json:"mentioned_by"`
}

// AuthorCount is one row of a top-authors breakdown.
type AuthorCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CitationBucket is one bucket of a citation-count histogram, e.g. "0", "1-5", "50+".
type CitationBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}
