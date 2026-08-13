## Purpose

Enables users to import PDF files from local disk into their Papeer library, extracting or manually providing metadata to create fully functional paper records that participate in the application's analysis, summarization, and organization workflows.

## ADDED Requirements

### Requirement: User selects PDF files for import

The system SHALL allow the user to select one or more PDF files from the local filesystem via a native file dialog.

#### Scenario: Single file selection
- **WHEN** user clicks the "Import PDF" button and selects one PDF file in the native dialog
- **THEN** the system initiates import for that single file and displays a metadata review interface

#### Scenario: Multiple file selection
- **WHEN** user selects 3 PDF files in the native file dialog
- **THEN** the system initiates import for all selected files and presents them in a batch import queue

#### Scenario: Non-PDF file rejected
- **WHEN** user selects a `.docx` file in the file dialog (if not pre-filtered)
- **THEN** the system SHALL reject the file with an error message indicating only PDF is supported

### Requirement: System extracts metadata from PDF

The system SHALL attempt to extract DOI, title, and author information from the PDF's embedded metadata (XMP and Info dictionary).

#### Scenario: DOI found in metadata
- **WHEN** a PDF contains a valid DOI in its XMP or Info metadata
- **THEN** the system extracts the DOI and uses it to look up full metadata via external API

#### Scenario: Title and authors found, no DOI
- **WHEN** a PDF contains title and author metadata but no DOI
- **THEN** the system pre-fills those fields in the metadata form for user review

#### Scenario: No metadata found
- **WHEN** a PDF contains no extractable metadata
- **THEN** the system presents an empty metadata form for manual entry

### Requirement: System resolves metadata via external API

When a DOI is extracted from the PDF, the system SHALL query Crossref API to retrieve full bibliographic metadata (title, authors, year, venue/journal, abstract).

#### Scenario: Successful DOI resolution
- **WHEN** a valid DOI is extracted and Crossref returns a successful response
- **THEN** the system pre-fills all available fields in the metadata form and marks the paper as DOI-matched

#### Scenario: DOI not found in Crossref
- **WHEN** a DOI is extracted but Crossref returns 404 or no data
- **THEN** the system still pre-fills the DOI field but leaves other metadata fields empty for manual entry

#### Scenario: API timeout or network error
- **WHEN** Crossref API is unreachable or times out
- **THEN** the system shows a warning that online lookup failed and presents the form with only locally extracted metadata pre-filled

### Requirement: User reviews and edits metadata

The system SHALL present a metadata form allowing the user to review, edit, or manually enter bibliographic information before creating the paper record.

#### Scenario: User confirms auto-filled metadata
- **WHEN** metadata was successfully auto-filled from API lookup and user clicks "Import"
- **THEN** the paper record is created with the confirmed metadata and the PDF is copied to the profile directory

#### Scenario: User edits incomplete metadata
- **WHEN** some fields are empty after extraction and the user fills them in manually before clicking "Import"
- **THEN** the paper record is created with the combined auto-filled and manual metadata

#### Scenario: User cancels import
- **WHEN** user clicks "Cancel" on the metadata form
- **THEN** no paper record is created and no file is copied; the import is fully aborted

#### Scenario: Required field missing
- **WHEN** user clicks "Import" without providing a title (required field)
- **THEN** the system SHALL show a validation error and not proceed until the title is provided

### Requirement: PDF file is copied to profile directory

The system SHALL copy the imported PDF into the active profile's PDF directory, using the same storage model as downloaded papers.

#### Scenario: Successful file copy
- **WHEN** paper record creation succeeds
- **THEN** the PDF is copied to the profile's PDF directory with a unique filename derived from the paper ID

#### Scenario: Source file no longer accessible
- **WHEN** the original PDF file has been deleted or moved between selection and copy attempt
- **THEN** the system SHALL show an error and abort the import

#### Scenario: Duplicate filename in profile directory
- **WHEN** copying the PDF would result in a filename collision in the profile directory
- **THEN** the system SHALL generate a unique filename (e.g., by appending a suffix)

### Requirement: Paper record is created in database

The system SHALL create a paper record in the `papers` table linked to the active search profile, with status set to `approved` and a flag distinguishing it as an imported paper.

#### Scenario: Successful record creation
- **WHEN** all metadata is confirmed and PDF is copied
- **THEN** a new row in `papers` is created with status `approved`, the source file path, and the DOI (if available); the paper appears in the papers list immediately

#### Scenario: Record creation for paper with multiple axes
- **WHEN** the active profile has multiple research axes
- **THEN** the imported paper is associated with all axes of the profile (same behavior as search-discovered papers)

### Requirement: System handles duplicate papers

The system SHALL detect when an imported PDF matches an existing paper record by DOI and attach the PDF to the existing record instead of creating a duplicate.

#### Scenario: DOI matches existing paper without PDF
- **WHEN** an imported PDF has a DOI that matches an existing paper that has no downloaded PDF
- **THEN** the system attaches the PDF to the existing record without creating a new paper entry

#### Scenario: DOI matches existing paper that already has PDF
- **WHEN** an imported PDF has a DOI that matches an existing paper that already has a downloaded PDF
- **THEN** the system SHALL notify the user about the conflict and offer options: replace the existing PDF, keep both, or cancel

#### Scenario: No DOI — new record always created
- **WHEN** an imported PDF has no DOI (or extraction failed)
- **THEN** the system creates a new paper record without deduplication checks (user is responsible for avoiding manual duplicates)

### Requirement: Citation data fetched for DOI-matched papers

For papers successfully matched by DOI, the system SHALL attempt to fetch citation data (citation count, citation links) via Semantic Scholar API to populate the citation graph.

#### Scenario: Citation data retrieved successfully
- **WHEN** a paper is imported with a DOI and Semantic Scholar returns citation data
- **THEN** the system populates `citation_count` and, if available, creates `citation_links` entries for the paper

#### Scenario: Citation data not available
- **WHEN** Semantic Scholar API returns no data for the given DOI
- **THEN** the paper is imported successfully without citation data; no error is shown to the user
