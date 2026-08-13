# Code Review: paper-triage-ux

## Summary

**Files changed:** 8 (+2 auto-generated bindings)
**Tests:** Go — all pass (14 packages). Frontend — all pass (41 tests).
**Compilation:** Go `go build ./...` OK. Vue `vue-tsc --noEmit` OK.

---

## Issues Found & Fixed During Review

### Issue 1: `engine.go` indentation (FIXED)

Export of `downloadOne` → `DownloadOne` broke indentation inside `DownloadAll` goroutine loop. Fixed with proper tab alignment.

### Issue 2: `progressStore.downloading` never set to `true` (FIXED)

**Symptom:** `BackgroundTasksIndicator`'s download progress card (`v-if="progressStore.downloading"`) and badge count were dead — the header never showed download activity.

**Root cause:** `progressStore.downloading` was only set to `true` in `DownloadModal.vue:311` before calling `DownloadApproved`. After removing the modal trigger from PapersView, nothing sets this flag. The `download:done` handler sets it to `false`, so it's permanently `false`.

**Fix (file: `stores/progress.ts`, line 107):**
```ts
EventsOn('download:progress', (event: DownloadEvent) => {
  if (!downloading.value) {
    resetDownload()
    downloading.value = true
  }
  // ... rest of handler
})
```
First progress event triggers reset + flag. `download:done` clears it. Works for both single-paper (`DownloadPaper`) and batch (`DownloadApproved`).

**Known limitation:** If a single-paper download and batch download run concurrently, `download:done` from the first-to-finish will set `downloading = false` prematurely, and the next progress event from the still-running download will reset the progress display. This is a cosmetic race condition that resolves itself within one event cycle. A proper fix would require operation-level tracking (same architectural issue as the cancel conflict below).

---

## Backend (`internal/`)

### `internal/download/engine.go` — Export `downloadOne` → `DownloadOne`

**Status:** OK. Idempotency already exists (line 146: `os.Stat(dest)` + `ValidatePDFFile`).

### `internal/app/app.go` — `DownloadPaper(paperID int64)`

**Status:** Works correctly.

**Concern: cancel conflicts with concurrent operations.**  
`DownloadPaper` calls `a.setCancel(cancel)` which replaces the entire App-level cancel function. If a batch `DownloadApproved` is already running, calling `DownloadPaper` will overwrite the batch download's cancel. When the single-paper download finishes, `a.clearCancel()` sets `a.cancel = nil`, making `CancelOperation()` impossible for the batch download.

This is a **pre-existing design issue** in the App struct — `Search`, `DownloadApproved`, `CitationFetch`, `ReviewDraft`, and now `DownloadPaper` all share a single `cancel` field. NOT a regression introduced by this change; it's an existing limitation now more likely to be triggered by auto-download.

### `internal/db/download.go` — `GetFailedDownloadPaperIDs`

**Status:** OK. SQL query returns papers with ≥1 failed download and 0 successful downloads. No index on `downloads(paper_id, status)` but per-paper download count is tiny.

### `internal/app/app.go` — `GetFailedDownloadPaperIDs`

**Status:** OK. Thin wrapper delegating to DB layer.

---

## Frontend (`frontend/src/`)

### `frontend/src/views/PapersView.vue` — Auto-advance logic

**Status:** OK. `setStatus` computes next index optimistically before re-fetch. `isAdvancing` flag suppresses watcher deselection during update. Edge cases (last item, empty list, "all" tab) handled.

### `frontend/src/views/PapersView.vue` — `download:done` listener

**Status:** OK. Guard `downloadDoneRegistered` prevents duplicate registration on remount. Listener persists for app lifetime (consistent with other components that don't call `EventsOff` for persistent events).

### `frontend/src/views/PapersView.vue` — Failed download badge

**Status:** OK. Orange chip shown when paper is `approved` + in `failedDownloadPaperIDs` set. Re-fetched on every `download:done` event via `loadStatusCounts` → `loadFailedDownloads`.

### `frontend/src/components/PaperDetailPanel.vue` — Contextual buttons

**Status:** OK. `showApproveButton` / `showRejectButton` computed from `statusFilter` prop. Correctly hides Approve on "approved"/"downloaded" tabs and Reject on "rejected" tab.

### `frontend/src/i18n/` — Translations

**Status:** OK. Added keys (`downloadAllApproved`, `colStatus`, `downloadFailed`). Updated shortcuts bar and onboarding text. Removed stale `downloadPdf` key — no remaining references.

---

## Wails Bindings (`frontend/wailsjs/`)

`App.d.ts` and `App.js` — manually added `DownloadPaper` and `GetFailedDownloadPaperIDs` declarations. These will be regenerated correctly on next `wails dev`/`wails build`.

---

## Recommendations

1. **Consider multiple-cancel support** (follow-up): Per-operation cancellation (`cancelMap` keyed by operation type) would fix both the cancel race and the progress indicator race for concurrent downloads.
