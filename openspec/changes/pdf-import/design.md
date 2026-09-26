## Context

Papeer is a desktop application (Wails v2 + Go backend + Vue 3 frontend) for systematic literature review. Papers are currently discovered exclusively through the search engine (OpenAlex, Crossref, SemanticScholar, arXiv). The `papers` table stores all records; PDFs are saved to a user-configurable `pdf_dir` in each search profile with download attempts logged in the `downloads` table. The existing `GetPaperPDFPath` method joins `downloads` to resolve on-disk file paths — any code that reads PDFs (viewer, summarization) relies on this path.

See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:**
- Users can select one or more PDF files from local disk and import them
- Automatic metadata extraction from PDF XMP/Info fields + DOI lookup via Crossref `/works/{DOI}`
- Manual metadata entry as fallback when automatic extraction fails
- Imported PDFs are copied to the profile's `pdf_dir` and registered in `downloads` so all existing PDF features (viewing, summarization) work without changes
- Basic deduplication: DOI match merges into existing paper; no DOI = new record always

**Non-Goals:**
- Batch import of entire directories (initial version supports multi-file selection only)
- OCR for scanned PDFs without text layer
- Import of non-PDF formats (EPUB, HTML, etc.)
- Bi-directional sync — files are copied into profile dir, original is left untouched

## Decisions

### 1. Schema: add `is_imported` column to papers

**Decision:** Add `is_imported INTEGER NOT NULL DEFAULT 0` to the `papers` table.

**Rationale:** The existing `status` CHECK constraint allows `('new','approved','rejected','downloaded')`. Imported papers are effectively `approved` + have a PDF, so `downloaded` status fits. However, we need to distinguish imported papers in the UI (e.g., a badge or icon) and for future features (re-import, export filters). A boolean column is the simplest discriminator.

**Alternatives considered:**
- Using a sentinel value in `sources` array (e.g., `["import"]`): fragile, easy to break on merge logic
- Adding `imported` to the status CHECK: requires migration touching existing rows; `downloaded` already covers the state
- Separate table for imported papers: overkill for a single boolean flag

### 2. PDF storage: copy to profile dir, record in downloads

**Decision:** Copy the selected PDF to `{profile.pdf_dir}/{paperID}.pdf` and create a `downloads` row with `source='import'` and `status='ok'`.

**Rationale:** The entire PDF pipeline — `GetPaperPDFPath`, `GenerateSummary`, `PdfViewer` — already works through the `downloads` table. By recording imports the same way, zero changes are needed to those subsystems. The paper status is set to `downloaded` to match the convention used by the download engine.

**Filename convention:** `{paperID}.pdf` — simple, collision-free (paper IDs are unique), and makes it obvious which paper a file belongs to.

**Edge case:** If source file is deleted between dialog selection and copy, abort with error. If destination already exists (shouldn't happen with ID-based naming), append `-N` suffix.

### 3. Metadata extraction: read XMP + Info dict, then Crossref DOI lookup

**Decision:** Two-phase extraction:
1. **Phase 1 (local):** Read PDF metadata stream — try XMP (XML-based, modern) first, fall back to Info dictionary (legacy). Extract DOI, title, authors.
2. **Phase 2 (remote):** If DOI found, query `https://api.crossref.org/works/{DOI}` to retrieve full bibliographic data using the existing `httpclient.Client` infrastructure.

**Rationale:** Crossref has the most complete and structured metadata. The project already has Crossref API integration in `internal/search/crossref.go` with rate limiting configured. The DOI-specific endpoint (`/works/{DOI}`) is simpler than the search endpoint and returns a single result. For phase 1, reading PDF metadata is lightweight (no parsing of the full document content) and the PDF library used for text extraction (`ledongthuc/pdf`) can be extended.

**Metadata precedence:** Crossref data overrides locally extracted data when both exist (Crossref is more structured and complete). User edits override everything.

### 4. Backend: new methods on App struct + lightweight import package

**Decision:** Add exported methods on the existing `App` struct in `internal/app/` for the import workflow. Metadata extraction logic lives in a new `internal/import/` package to keep `app.go` focused on coordination.

**Exported methods:**
```
ImportSelectFiles() ([]string, error)            // native file dialog
ExtractPDFMetadata(filePath string) (*ImportMetadata, error)  // phase 1+2
ImportPaper(filePath string, metadata ImportMetadata) (*db.Paper, error)  // full import
```

**Rationale:** The `App` struct is the Wails bindings facade — all methods callable from the frontend must be on it. Extraction logic is independent enough to warrant its own package, avoiding further bloat of the already 1700-line `app.go`.

### 5. Frontend: import modal in PapersView toolbar

**Decision:** Add an "Import PDF" button to the PapersView toolbar area. Click opens a two-step modal:
1. **Step 1 — Progress:** Shows file selection progress, metadata extraction status per file
2. **Step 2 — Review:** For each file, shows extracted metadata in editable fields. User can confirm, edit, or skip individual files

**Rationale:** Multi-step modal keeps the flow contained. Batch review with editable fields handles the common case (auto-extracted metadata needs tweaks) and the fallback case (manual entry). The modal pattern is already used for downloads (`DownloadModal.vue`).

### 6. Duplicate handling: leverage UpsertPaper + explicit conflict resolution

**Decision:** Before creating a new record, check for existing paper by DOI. If found:
- No existing PDF → attach PDF to existing record, update status to `downloaded`
- Existing PDF already → prompt user: replace / keep both / cancel
- No DOI → create new record (no dedup check by title to avoid false matches on short titles from manual entry)

**Rationale:** The existing `UpsertPaper` does DOI + title matching, but it was designed for search results where titles are more reliable. For manual imports, title-only dedup is risky (e.g., "Paper" or "Untitled"). DOI-based dedup is deterministic and safe.

## Risks / Trade-offs

- **[Risk]** Extracted DOI from PDF metadata may be malformed → **Mitigation:** Validate DOI format with regex before Crossref lookup; show raw extracted value in form if invalid
- **[Risk]** User imports a large number of PDFs and Crossref rate limits kick in → **Mitigation:** The `httpclient` already enforces per-host rate limits (5 req/s for api.crossref.org); batch import should process sequentially with visible progress
- **[Risk]** `pdftotext` fallback for text extraction may not be available on all platforms → **Mitigation:** Metadata extraction does NOT require `pdftotext`; XMP/Info reading is pure Go. Summarization will still work since text extraction already handles the fallback
- **[Trade-off]** Imported papers won't have axis associations unless explicitly assigned → Acceptable: imported papers are not tied to search axes; the UI should show them under "All" papers. The `paper_axes` table will not have entries for imported papers unless the user manually assigns them.
- **[Trade-off]** Deduplication is DOI-only, not content-hash → Acceptable for MVP. Content fingerprinting (e.g., hash of first N pages) could be added later for papers without DOI.
