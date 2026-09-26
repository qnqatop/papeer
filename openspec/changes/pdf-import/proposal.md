## Why

Papeer currently requires all papers to be discovered through its search engine, but researchers often already have PDF collections on disk — previously downloaded articles, papers from colleagues, preprints not yet indexed. These PDFs are invisible to Papeer's analysis, summarization, and organization features. Adding local PDF import closes this gap and makes Papeer useful from day one, even before running a single search.

## What Changes

- Add ability to select and import one or more PDF files from local disk via native file dialog
- Extract DOI and basic metadata (title, authors) from PDF XMP/Info metadata
- Resolve metadata via external APIs (Crossref) when a valid DOI is found
- Allow manual metadata entry as fallback when automatic extraction fails or finds nothing
- Copy imported PDFs into the profile's PDF directory (same storage model as downloaded papers)
- Create paper records in the database with status `approved`, linked to the active profile
- Provide basic deduplication: if a PDF matches an existing paper by DOI, attach the PDF to that record; if no DOI match, create a new record
- For papers successfully matched by DOI, optionally fetch citation data via Semantic Scholar API

## Capabilities

### New Capabilities

- `pdf-import`: local PDF file selection, metadata extraction, DOI-based lookup, manual metadata entry, PDF copying to profile directory, paper record creation, and deduplication against existing papers

### Modified Capabilities

<!-- No existing capabilities modified — this is a net-new feature. -->

## Impact

- **Database**: new columns or status value for papers table to distinguish imported papers; `downloads` table may get entries for imported PDFs
- **Backend**: new exported methods on `App` struct for file dialog, metadata extraction, DOI lookup, paper creation; new package or additions to existing packages (`internal/app/`, possibly a new `internal/import/` package)
- **Frontend**: new UI components (import button, import modal/dialog with metadata form, batch import progress); new composable for import state
- **i18n**: new translation keys in `en.json` and `ru.json`
- **Dependencies**: Go library for reading PDF metadata/XMP (may already be present via `ledongthuc/pdf` or needs a new one); native file dialog already available via Wails runtime
