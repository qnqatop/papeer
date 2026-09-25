## Why

First users report that the paper triage workflow is clunky: after approving or rejecting a paper, the next one does not open automatically — they must manually click or press keys. Downloads feel blocking because they open a centered modal that discourages closing it. The action buttons (approve/reject) are shown uniformly regardless of the current filter context, and paper status is invisible in the list. These friction points make the core curation loop — the most frequent user activity — feel slow and confusing.

## What Changes

- **Auto-advance after action**: When a user approves or rejects a paper, the selection automatically moves to the next paper in the filtered list; the paper disappears if it no longer matches the current filter
- **Contextual action buttons**: When viewing papers filtered to "approved" or "downloaded", only the Reject button is shown; when filtered to "rejected", only the Approve button; when filtered to "new" or "all", both buttons are shown
- **Status badges in paper list**: Each row in the paper table shows a colored status indicator (new blue, approved green, rejected red, downloaded purple)
- **Background auto-download**: After approving a paper, its PDF download starts automatically in the background without opening a modal; progress is visible in the header indicator
- **DownloadModal replaced with non-blocking indicator**: The blocking download modal is replaced by the existing header-based `BackgroundTasksIndicator` with an expandable detail panel; manual "Download All" remains available
- **Single-paper download**: A new `DownloadPaper` backend method downloads one specific paper, used by the auto-download trigger
- **Onboarding update**: Driver.js tour steps for PapersView are adjusted to reflect the new UI elements and behavior

## Capabilities

### New Capabilities

- `paper-triage-ux`: auto-advance after approve/reject, contextual action buttons, status badges in list, background single-paper auto-download, non-blocking download progress

### Modified Capabilities

<!-- No existing capabilities modified — net-new UX behavior. -->

## Impact

- **Frontend**: `PapersView.vue` (auto-advance logic, action button visibility, status badges, download trigger changes), `PaperDetailPanel.vue` (contextual button visibility based on filter), `DownloadModal.vue` (replaced or significantly reduced in scope), `BackgroundTasksIndicator.vue` (enhanced with expandable detail), `PaperSummary.vue` (summary visibility no longer tied to modal), `stores/papers.ts` and `stores/progress.ts` (new event handling for single-paper download), onboarding steps
- **Backend**: New `DownloadPaper(paperID)` method on `App` struct; download engine must support single-paper downloads without full batch fetch
- **i18n**: New/changed keys for status badge labels, download progress text, updated onboarding strings
- **No database changes required**
