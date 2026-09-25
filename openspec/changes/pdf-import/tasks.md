## 1. Database schema

- [ ] 1.1 Add `is_imported INTEGER NOT NULL DEFAULT 0` column to `papers` table in `internal/db/sqlite.go`
- [ ] 1.2 Add `is_imported` field to the `Paper` struct in `internal/db/models.go`
- [ ] 1.3 Update `GetPaper`, `ListPapers`, `insertPaper`, and all other `SELECT`/`INSERT` queries that touch `papers` to include `is_imported`
- [ ] 1.4 Write migration test: verify existing papers have `is_imported=0`, new imported papers have `is_imported=1`

## 2. Metadata extraction (internal/import package)

- [ ] 2.1 Create `internal/import/` package with `ImportMetadata` struct (Title, Authors, DOI, Year, Venue, Abstract)
- [ ] 2.2 Implement `ExtractMetadata(filePath string) (*ImportMetadata, error)` — read XMP metadata stream from PDF; fall back to Info dictionary; extract DOI, title, authors
- [ ] 2.3 Add DOI format validation (regex) to filter out malformed identifiers before API lookup
- [ ] 2.4 Implement `LookupDOI(doi string, client *httpclient.Client) (*ImportMetadata, error)` — query `https://api.crossref.org/works/{DOI}`, parse response into `ImportMetadata`
- [ ] 2.5 Implement combined extraction: `ExtractAndResolve(filePath string, client *httpclient.Client) (*ImportMetadata, error)` — call ExtractMetadata, if DOI found call LookupDOI, merge results (Crossref wins on conflict)
- [ ] 2.6 Write unit tests for metadata extraction (PDF with XMP, PDF with Info only, PDF with no metadata, malformed DOI)

## 3. Backend: App methods for import

- [ ] 3.1 Add `ImportSelectFiles() ([]string, error)` — call `runtime.OpenFileDialog` with PDF filter, return selected paths
- [ ] 3.2 Add `ConfirmImport(filePaths []string) ([]ImportPreview, error)` — for each file: call `import.ExtractAndResolve`, return preview data to frontend. Handle per-file errors (non-PDF, unreadable) without failing the whole batch
- [ ] 3.3 Add `ImportPaper(filePath string, metadata ImportMetadata) (*db.Paper, error)` — full import flow:
  - Check for existing paper by DOI; if found with no PDF → attach; if found with PDF → return conflict error so frontend can prompt user
  - Create paper record via `insertPaper` with `is_imported=1`, `status='downloaded'`
  - Copy PDF to `{profile.pdf_dir}/{paperID}.pdf` (create dir if needed, handle source-not-found)
  - Create `downloads` row with `source='import'`, `status='ok'`, `filename='{paperID}.pdf'`
  - Fallback: if no DOI, create new record without dedup check
- [ ] 3.4 Add `ResolveImportConflict(paperID int64, action string) (*db.Paper, error)` — handle replace/keep-both/cancel for existing-PDF conflict
- [ ] 3.5 Add `ImportFetchCitations(paperID int64) error` — call Semantic Scholar API by DOI to populate `citation_count` and `citation_links` (reuse existing `CitationFetcher` or add lightweight helper). Fire-and-forget — failure should not block import
- [ ] 3.6 Write integration tests for ImportPaper (new paper, DOI duplicate, missing source file)

## 4. Frontend: import modal

- [ ] 4.1 Add "Import PDF" button to PapersView toolbar (next to existing action buttons)
- [ ] 4.2 Create `ImportModal.vue` component with two-step flow:
  - Step 1 (File selection): call `ImportSelectFiles()`, show selected files with loading spinners while metadata extracts, display extraction status per file
  - Step 2 (Review): for each file, show editable form (title*, authors, year, venue, DOI, abstract). Pre-filled from extraction. Validate title is non-empty before allowing import
- [ ] 4.3 Implement batch confirm — user clicks "Import All" → calls `ImportPaper` for each confirmed file; shows success/failure per item
- [ ] 4.4 Handle duplicate conflicts: modal for "PDF already exists for this paper" with replace/keep both/cancel options
- [ ] 4.5 After successful import, refresh papers list and show notification with count
- [ ] 4.6 Add `is_imported` badge/indicator in paper list rows and PaperDetailPanel

## 5. i18n

- [ ] 5.1 Add English translation keys (`en.json`) for: import button, modal titles, form labels, status messages, errors
- [ ] 5.2 Add Russian translation keys (`ru.json`) for the same set

## 6. End-to-end validation

- [ ] 6.1 Manual test: import single PDF with valid DOI → verify paper appears, PDF is viewable, summary can be generated
- [ ] 6.2 Manual test: import PDF with no metadata → verify manual entry form works, paper is created
- [ ] 6.3 Manual test: import PDF matching existing paper by DOI → verify dedup and conflict resolution flow
- [ ] 6.4 Manual test: import 3+ PDFs in batch → verify all processed correctly
- [ ] 6.5 Verify imported papers appear correctly in stats, export, and analysis views
