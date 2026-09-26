## Context

Papeer's `PapersView` is a single-page view combining a filterable paper table (left panel) and a detail panel (right panel). Status changes trigger a full re-fetch via the Pinia store, which currently deselects the paper and returns the detail panel to its empty state. The `DownloadModal` is a centered Naive UI modal that, while technically closable, appears blocking and discourages continued triage. Downloads already run in a background goroutine and emit Wails events — the infrastructure for non-blocking downloads exists.

Keyboard shortcuts (`j`/`k`/`a`/`r`/`Enter`) are registered on `document` with capture phase. The `PaperDetailPanel` shows both Approve and Reject buttons unconditionally. No status indicator exists in the table rows (only opacity for rejected papers). The `BackgroundTasksIndicator` in the header already shows download progress for batch downloads.

See `proposal.md` for motivation and `specs/paper-triage-ux/spec.md` for requirements.

## Goals / Non-Goals

**Goals:**
- Auto-select the next paper after any status change, without user intervention
- Show/hide action buttons based on current status filter context
- Add colored status badges to every table row
- Trigger single-paper download immediately after approve, in background
- Replace the blocking `DownloadModal` with a non-blocking header-based progress display
- Provide a `DownloadPaper(id)` backend method for single-paper downloads

**Non-Goals:**
- Changing the download engine's fallback chain or source resolvers
- Modifying the PDF viewer or summarization flow
- Changing the keyboard shortcut bindings (keys remain `j`/`k`/`a`/`r`)
- Full redesign of the PapersView layout — keep the existing split-panel structure
- Download queue management (pause/resume/reorder)

## Decisions

### 1. Auto-advance: optimistic selection before re-fetch

**Decision:** After calling `setStatus`, immediately compute the next index in the current list *before* the re-fetch returns, apply it optimistically, then re-fetch. If the acted-on paper is still in the new list (e.g., on "all" tab), advance to the next one. If it disappeared (e.g., on "new" tab), the same index now points to the paper that took its place.

**Implementation:**
```js
const oldIndex = selectedIndex.value
store.setStatus(paper.id, status)  // triggers fetchPapers() async

if (statusFilter.value === '' || statusFilter.value === paper.status) {
  // Paper won't disappear — advance to next
  selectedIndex.value = Math.min(oldIndex + 1, papers.value.length - 1)
} else {
  // Paper will disappear — keep current index (next paper fills the slot)
  // If it was the last item, move to new last
  if (oldIndex >= papers.value.length - 1) {
    selectedIndex.value = Math.max(0, papers.value.length - 2)
  } else {
    selectedIndex.value = oldIndex
  }
}
```

**Rationale:** We can predict whether the paper will disappear based on the current filter. No need to wait for the re-fetch. This avoids flicker where the detail panel briefly shows empty state.

**Alternative considered:** Wait for `fetchPapers()` Promise, then find the paper by ID in the new list. Slower, causes visible empty state between action and advance.

### 2. Contextual buttons: pass filter to PaperDetailPanel

**Decision:** Add a `statusFilter` prop to `PaperDetailPanel`. The component uses it to decide button visibility:

| Filter | Approve | Reject |
|--------|---------|--------|
| `"new"` | visible | visible |
| `""` (all) | visible | visible |
| `"approved"` | hidden | visible |
| `"rejected"` | visible | hidden |
| `"downloaded"` | hidden | visible |

**Rationale:** The filter context already exists in `PapersView.vue` as `statusFilter`. Passing it as a prop is simpler than introducing a new state or deriving it from paper status (which would be wrong — a paper on "all" tab might have any status).

**Alternative considered:** Deriving button visibility from the paper's own status alone. Rejected: an approved paper on the "all" tab should show both buttons, not just reject.

### 3. Status badges: colored NTag before title

**Decision:** Add an `NTag` component to the title column in the data table, with these mappings:

| Status | Color | Label (EN/RU) |
|--------|-------|---------------|
| new | `#3b82f6` (blue) | new / новый |
| approved | `#22c55e` (green) | approved / одобрено |
| rejected | `#ef4444` (red) | rejected / отклонено |
| downloaded | `#8b5cf6` (purple) | downloaded / скачано |

**Rationale:** Naive UI's `NTag` is already used in the score column. Consistent visual language. Users immediately see each paper's state without opening the detail panel. The existing `opacity: 0.45` for rejected papers is kept as a secondary visual cue.

### 4. Single-paper download: DownloadPaper method

**Decision:** Add `DownloadPaper(paperID int64) error` to the `App` struct. It loads the paper and profile, creates a download engine, and runs `downloadOne` directly (bypassing the worker pool since there's only one job). Emits the same `download:progress` / `download:done` events as the batch method.

The existing `engine.downloadOne` method already handles a single paper — the new method just needs to invoke it without the `DownloadAll` worker pool wrapper.

```go
func (a *App) DownloadPaper(paperID int64) error {
    paper, _ := a.db.GetPaper(paperID)
    profile, _ := a.db.GetProfile(paper.ProfileID)
    // ... setup engine, call downloadOne in goroutine
}
```

**Rationale:** Reuses all existing download infrastructure. Single-paper download avoids the overhead of scanning all approved papers (which `GetApprovedPapers` does). The engine's `downloadOne` is already idempotent — it skips papers that already have a valid PDF on disk.

**Alternative considered:** Call `DownloadApproved` with a filter. Rejected: `GetApprovedPapers` fetches all approved papers, wasteful for single-paper trigger. Also, `DownloadApproved` blocks the single flow with batch semantics.

### 5. Background auto-download trigger

**Decision:** In `PapersView.setStatus()`, after changing status to "approved" (or toggling back from rejected to approved), call `DownloadPaper(paperID)` without awaiting. The frontend does not track or display per-paper download state — the header indicator handles that globally.

```js
async function setStatus(paper, status) {
    await store.setStatus(paper.id, status)
    // ... auto-advance logic ...
    if (status === 'approved') {
        DownloadPaper(paper.id)  // fire and forget
    }
}
```

**Rationale:** Fire-and-forget keeps the triage flow fast. The `progressStore` already listens for `download:progress` events globally, so the header indicator will show activity regardless of who triggered the download.

**Edge case:** If user rapidly approves 5 papers, 5 `DownloadPaper` calls fire. The download engine re-initializes for each. This is acceptable for now; a future optimization could batch them.

### 6. Download modal → header indicator

**Decision:** Remove `DownloadModal` as the primary download UI. Replace with:
- **Download banner** in PapersView stays, but "Download All" text changes to "Download {N} Approved" and clicking directly calls `DownloadApproved` without a confirmation modal
- **Header indicator** (`BackgroundTasksIndicator`) is enhanced: clicking it opens a popover with per-paper download log (reusing the log card UI from the old modal)
- The `DownloadModal` component is kept but simplified — only used when the user clicks to view details on a specific download, not as the default trigger

**Rationale:** The modal was the root cause of the "stuck waiting" feeling. Moving progress to the header makes it truly non-blocking. The old modal's per-paper log cards are valuable — they move to the header popover.

**Alternative considered:** Keep the modal but add a "minimize to background" button. Rejected: adds complexity without fixing the core problem (users still feel they need to wait).

## Risks / Trade-offs

- **[Risk]** Rapid sequential approvals could spawn many concurrent `DownloadPaper` goroutines → **Mitigation:** The download engine is lightweight (just calls `downloadOne`), and the HTTP client already rate-limits per host. If performance becomes an issue, add a channel-based queue in a follow-up.
- **[Risk]** Auto-advance calculation is wrong when the re-fetch returns different data than expected (e.g., another concurrent action changed the paper) → **Mitigation:** After re-fetch, validate that the selected paper still exists in the list; if not, recalculate index based on the actual returned data.
- **[Trade-off]** Removing the download confirmation modal means users can no longer see the target directory or paper count before downloads start → Acceptable: the auto-download is for a single paper (low stakes). The "Download All" button still shows the count on the banner label. Target directory is configured in Settings.
- **[Trade-off]** Status badges add visual noise to the table → Acceptable: badges are small (compact NTag) and users of literature review tools expect to see paper status at a glance.
