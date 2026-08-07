package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection with application-specific methods.
type DB struct {
	*sql.DB
}

// NewDB opens (or creates) a SQLite database at path and runs migrations.
func NewDB(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Single connection for SQLite — avoids locking issues.
	sqlDB.SetMaxOpenConns(1)

	d := &DB{sqlDB}

	if err := d.pragma(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}

	// The database stores LLM API keys in plaintext, so restrict the file to
	// the owner (0600). Done after pragma so the file exists on disk (sql.Open
	// is lazy). Best-effort: ignore errors on in-memory DBs or platforms where
	// chmod is a no-op.
	if path != "" && path != ":memory:" {
		_ = os.Chmod(path, 0o600)
	}

	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

func (d *DB) pragma() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := d.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

func (d *DB) migrate() error {
	if _, err := d.Exec(schema); err != nil {
		return err
	}
	// Add columns that may not exist in older databases.
	// Ignore "duplicate column" errors.
	d.Exec("ALTER TABLE axes ADD COLUMN last_radar_run DATETIME")
	d.Exec("ALTER TABLE papers ADD COLUMN ai_match_score INTEGER DEFAULT 0;")
	d.Exec("ALTER TABLE papers ADD COLUMN s2_paper_id TEXT;")
	// last_citation_fetch_at marks when a paper was last processed by the
	// citation fetch worker (resolved or not — an attempt was made). Used to
	// compute an accurate papers_processed count and to resume a cancelled
	// fetch without redoing already-processed papers.
	d.Exec("ALTER TABLE papers ADD COLUMN last_citation_fetch_at DATETIME;")

	// Older DBs have keywords.type CHECK without 'exclude'. SQLite cannot
	// ALTER a CHECK constraint, so rebuild the table when needed.
	if err := d.migrateKeywordExclude(); err != nil {
		return fmt.Errorf("migrate keyword exclude: %w", err)
	}
	return nil
}

func (d *DB) migrateKeywordExclude() error {
	var sqlStr string
	if err := d.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='keywords'`,
	).Scan(&sqlStr); err != nil {
		// If keywords table is missing, schema CREATE above already used the new CHECK.
		return nil
	}
	if strings.Contains(sqlStr, "'exclude'") {
		return nil
	}

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE keywords_new (
            id      INTEGER PRIMARY KEY AUTOINCREMENT,
            axis_id INTEGER NOT NULL REFERENCES axes(id) ON DELETE CASCADE,
            word    TEXT NOT NULL,
            type    TEXT NOT NULL CHECK(type IN ('must','boost','exclude'))
        )`,
		`INSERT INTO keywords_new (id, axis_id, word, type) SELECT id, axis_id, word, type FROM keywords`,
		`DROP TABLE keywords`,
		`ALTER TABLE keywords_new RENAME TO keywords`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const schema = `
CREATE TABLE IF NOT EXISTS search_profiles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL DEFAULT '',
    pdf_dir         TEXT NOT NULL DEFAULT '',
    year_min        INTEGER NOT NULL DEFAULT 2018,
    vintage_year    INTEGER NOT NULL DEFAULT 2010,
    max_per_query   INTEGER NOT NULL DEFAULT 25,
    download_sources TEXT NOT NULL DEFAULT '["arxiv","semantic_scholar","openalex","unpaywall","crossref","s2_title"]',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS axes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id      INTEGER NOT NULL REFERENCES search_profiles(id) ON DELETE CASCADE,
    axis_key        TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    year_min        INTEGER,
    max_per_query   INTEGER,
    position        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(profile_id, axis_key)
);

CREATE TABLE IF NOT EXISTS queries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    axis_id         INTEGER NOT NULL REFERENCES axes(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,
    position        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS keywords (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    axis_id         INTEGER NOT NULL REFERENCES axes(id) ON DELETE CASCADE,
    word            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK(type IN ('must', 'boost', 'exclude'))
);

CREATE TABLE IF NOT EXISTS papers (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id        INTEGER NOT NULL REFERENCES search_profiles(id),
    axis_id           INTEGER REFERENCES axes(id),
    title             TEXT NOT NULL,
    title_normalized  TEXT NOT NULL,
    abstract          TEXT NOT NULL DEFAULT '',
    year              INTEGER,
    venue             TEXT NOT NULL DEFAULT '',
    authors           TEXT NOT NULL DEFAULT '[]',
    doi               TEXT,
    arxiv_id          TEXT,
    pdf_url           TEXT,
    pdf_source        TEXT,
    citation_count    INTEGER NOT NULL DEFAULT 0,
    pre_score         INTEGER NOT NULL DEFAULT 0,
    score_reasons     TEXT NOT NULL DEFAULT '[]',
    sources           TEXT NOT NULL DEFAULT '[]',
    status            TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new', 'approved', 'rejected', 'downloaded')),
    user_score        INTEGER CHECK(user_score BETWEEN 1 AND 5),
    notes             TEXT NOT NULL DEFAULT '',
    found_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_papers_profile_doi
    ON papers(profile_id, doi) WHERE doi IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_papers_profile_title
    ON papers(profile_id, title_normalized);

CREATE INDEX IF NOT EXISTS idx_papers_profile ON papers(profile_id);
CREATE INDEX IF NOT EXISTS idx_papers_status ON papers(profile_id, status);
CREATE INDEX IF NOT EXISTS idx_papers_score ON papers(profile_id, pre_score DESC);

CREATE TABLE IF NOT EXISTS downloads (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    paper_id        INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    source          TEXT NOT NULL,
    url             TEXT,
    status          TEXT NOT NULL CHECK(status IN ('ok', 'fail', 'skip')),
    reason          TEXT NOT NULL DEFAULT '',
    filename        TEXT,
    file_size       INTEGER,
    attempted_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_downloads_paper ON downloads(paper_id);

-- App-wide settings (proxy, LLM config, etc.)
CREATE TABLE IF NOT EXISTS app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- Tags for papers
CREATE TABLE IF NOT EXISTS tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER NOT NULL REFERENCES search_profiles(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#6366f1',
    UNIQUE(profile_id, name)
);

CREATE TABLE IF NOT EXISTS paper_tags (
    tag_id   INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    PRIMARY KEY (tag_id, paper_id)
);

CREATE INDEX IF NOT EXISTS idx_paper_tags_paper ON paper_tags(paper_id);

-- Track which axes a paper was found in (many-to-many)
CREATE TABLE IF NOT EXISTS paper_axes (
    paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    axis_id  INTEGER NOT NULL REFERENCES axes(id) ON DELETE CASCADE,
    PRIMARY KEY (paper_id, axis_id)
);

CREATE INDEX IF NOT EXISTS idx_paper_axes_paper ON paper_axes(paper_id);

-- Backfill paper_axes from existing papers.axis_id
INSERT OR IGNORE INTO paper_axes (paper_id, axis_id)
    SELECT id, axis_id FROM papers WHERE axis_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS summaries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    paper_id    INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    prompt_hash TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '',
    tokens_in   INTEGER NOT NULL DEFAULT 0,
    tokens_out  INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'generating', 'done', 'error')),
    error_msg   TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(paper_id, model)
);

CREATE INDEX IF NOT EXISTS idx_summaries_paper ON summaries(paper_id);
CREATE INDEX IF NOT EXISTS idx_summaries_status ON summaries(status);

-- Citation graph: links between papers in the database
CREATE TABLE IF NOT EXISTS citation_links (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id    INTEGER NOT NULL REFERENCES search_profiles(id) ON DELETE CASCADE,
    from_paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    to_paper_id   INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    UNIQUE(from_paper_id, to_paper_id)
);

CREATE INDEX IF NOT EXISTS idx_citation_links_from ON citation_links(from_paper_id);
CREATE INDEX IF NOT EXISTS idx_citation_links_to ON citation_links(to_paper_id);
CREATE INDEX IF NOT EXISTS idx_citation_links_profile ON citation_links(profile_id);

-- Citation graph: external papers not in user's DB but referenced by/citing known papers
CREATE TABLE IF NOT EXISTS external_citations (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id     INTEGER NOT NULL REFERENCES search_profiles(id) ON DELETE CASCADE,
    s2_paper_id    TEXT NOT NULL,
    title          TEXT NOT NULL,
    year           INTEGER,
    citation_count INTEGER NOT NULL DEFAULT 0,
    authors        TEXT NOT NULL DEFAULT '[]',
    mention_count  INTEGER NOT NULL DEFAULT 1,
    UNIQUE(profile_id, s2_paper_id)
);

CREATE INDEX IF NOT EXISTS idx_external_citations_profile ON external_citations(profile_id);

-- Links a specific internal paper to an external citation it mentions
-- (via a reference or a citing work). Without this table there is no way to
-- tell *which* of the user's papers pulled in a given external work — this
-- is what powers "who mentions this" in the Coverage Gaps tab.
CREATE TABLE IF NOT EXISTS citation_mentions (
    paper_id    INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    external_id INTEGER NOT NULL REFERENCES external_citations(id) ON DELETE CASCADE,
    PRIMARY KEY (paper_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_citation_mentions_external ON citation_mentions(external_id);
CREATE INDEX IF NOT EXISTS idx_citation_mentions_paper ON citation_mentions(paper_id);

-- LLM connection profiles. The API key is NOT stored here — it lives in the OS
-- keychain (service "papeer-llm", account = profile id).
CREATE TABLE IF NOT EXISTS llm_profiles (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    base_url      TEXT NOT NULL DEFAULT '',
    models        TEXT NOT NULL DEFAULT '[]',   -- JSON array of model names
    default_model TEXT NOT NULL,
    temperature   REAL NOT NULL DEFAULT 0.2,
    is_active     INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_profiles_active
    ON llm_profiles(is_active) WHERE is_active = 1;
`
