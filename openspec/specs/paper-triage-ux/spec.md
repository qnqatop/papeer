## Purpose

Improves the paper curation workflow by adding auto-advance after actions, contextual action buttons, status badges, and background auto-download — making the most frequent user activity (reviewing and classifying papers) fast and fluid without blocking modals.

## Requirements

### Requirement: Auto-advance after approve or reject

After the user approves or rejects a paper, the system SHALL automatically select the next paper in the filtered list; if the acted-on paper no longer matches the current status filter, it SHALL disappear from the list.

#### Scenario: Approve paper on "new" tab — auto-advance to next
- **WHEN** user approves paper #2 on the "new" tab (filters: status=new)
- **THEN** paper #2 disappears from the list (status changed to approved)
- **AND** the selection moves to what was paper #3 (now the new #2 position)

#### Scenario: Reject paper on "new" tab — auto-advance to next
- **WHEN** user rejects paper #1 on the "new" tab
- **THEN** paper #1 disappears, selection moves to what was paper #2

#### Scenario: Act on last paper in list — move to previous
- **WHEN** user approves the last paper in the current filtered view
- **THEN** the selection moves to the new last paper (what was previously second-to-last)

#### Scenario: Act on paper on "all" tab — advance without disappearing
- **WHEN** user approves paper #3 on the "all" tab (filters: status=all)
- **THEN** paper #3 remains visible (status changed but still matches "all" filter)
- **AND** selection advances to paper #4

#### Scenario: Act on paper with detail panel open — detail updates
- **WHEN** user presses `a` with paper details visible on the right panel
- **THEN** the detail panel updates to show the newly selected paper's details automatically

### Requirement: Contextual action buttons

The system SHALL show only the action buttons relevant to the current status filter context in the paper detail panel.

#### Scenario: Viewing "approved" papers — only reject shown
- **WHEN** the user is filtering papers by status "approved" and selects a paper
- **THEN** only the Reject button is shown (which changes status to "rejected")

#### Scenario: Viewing "rejected" papers — only approve shown
- **WHEN** the user is filtering papers by status "rejected" and selects a paper
- **THEN** only the Approve button is shown (which changes status to "approved")

#### Scenario: Viewing "new" papers — both buttons shown
- **WHEN** the user is filtering papers by status "new" and selects a paper
- **THEN** both Approve and Reject buttons are shown

#### Scenario: Viewing "all" papers — both buttons shown
- **WHEN** the user is filtering papers with no status filter ("all" tab)
- **THEN** both Approve and Reject buttons are shown, regardless of the paper's current status

### Requirement: Status badges in paper list

Each row in the paper table SHALL display a compact colored status indicator distinguishing new, approved, rejected, and downloaded papers.

#### Scenario: New paper shown with blue badge
- **WHEN** a paper has status "new"
- **THEN** the table row shows a blue dot or label indicating "new"

#### Scenario: Approved paper shown with green badge
- **WHEN** a paper has status "approved"
- **THEN** the table row shows a green indicator for "approved"

#### Scenario: Rejected paper shown with red badge
- **WHEN** a paper has status "rejected"
- **THEN** the table row shows a red indicator for "rejected" (in addition to reduced opacity)

#### Scenario: Downloaded paper shown with distinct badge
- **WHEN** a paper has status "downloaded"
- **THEN** the table row shows a purple or distinct indicator for "downloaded"

### Requirement: Background auto-download after approve

When a user approves a paper, the system SHALL automatically initiate a background download of its PDF for that single paper without blocking further interaction.

#### Scenario: Single paper auto-download triggered
- **WHEN** user approves a paper
- **THEN** a background download for that paper starts immediately
- **AND** the user can continue reviewing other papers without interruption

#### Scenario: Download already in progress for same paper
- **WHEN** user approves a paper whose download is already running
- **THEN** no duplicate download is started; the existing download continues

#### Scenario: Paper already downloaded — no action
- **WHEN** user approves a paper that already has a successfully downloaded PDF
- **THEN** no download is triggered

#### Scenario: Download fails silently
- **WHEN** the background download fails (e.g., no sources found)
- **THEN** the paper remains in "approved" status with no PDF
- **AND** the failure is visible only in the expanded download details, not as a blocking error

### Requirement: Non-blocking download progress

The system SHALL display download progress in a non-blocking header indicator instead of a centered modal, with expandable per-paper details available on demand.

#### Scenario: Download indicator visible during background downloads
- **WHEN** one or more background downloads are in progress
- **THEN** the header shows a compact download indicator with current/total count

#### Scenario: Expand download details
- **WHEN** user clicks the download indicator in the header
- **THEN** a popover or side panel shows per-paper download status with source attempts

#### Scenario: Download complete — brief notification
- **WHEN** all queued downloads finish
- **THEN** a brief non-blocking notification appears with success/failure counts
- **AND** the paper list refreshes to update statuses

#### Scenario: Manual "Download All" still available
- **WHEN** the user clicks a "Download All Approved" button in the Papers view
- **THEN** all approved papers without PDFs are queued for background download
- **AND** no blocking modal is shown; progress appears in the header indicator

### Requirement: Single-paper download backend method

The system SHALL provide a method to download the PDF for a single specified paper, distinct from the existing batch download method.

#### Scenario: Download single paper successfully
- **WHEN** `DownloadPaper` is called with a valid paper ID that has no PDF
- **THEN** the system runs the download fallback chain for that one paper
- **AND** emits `download:progress` and `download:done` events like the batch method

#### Scenario: Download single paper that already has a PDF
- **WHEN** `DownloadPaper` is called for a paper that already has a successful download
- **THEN** the method returns without starting a new download

### Requirement: Onboarding updated for new UX

The driver.js onboarding tour for the Papers view SHALL reflect the new UI elements: status badges, contextual action buttons, background download indicator, and auto-advance behavior.

#### Scenario: Onboarding highlights status badges
- **WHEN** the onboarding tour reaches the Papers view step
- **THEN** the tour highlights the status badge column and explains its meaning

#### Scenario: Onboarding mentions auto-advance
- **WHEN** the onboarding tour explains keyboard shortcuts
- **THEN** it mentions that after approve/reject, the view advances automatically
