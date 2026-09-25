## 1. Backend: single-paper download

- [x] 1.1 Expose `downloadOne` as a public method on the download `Engine` (or add a `DownloadPaper` wrapper in `internal/app/app.go` that loads paper+profile and calls `engine.downloadOne` in a goroutine)
- [x] 1.2 Ensure `DownloadPaper` emits `download:progress` and `download:done` events with the same format as `DownloadApproved`
- [x] 1.3 Add idempotency guard: if paper already has a valid PDF on disk, return immediately without starting download
- [x] 1.4 Register `DownloadPaper` in Wails bindings so frontend can call it

## 2. Auto-advance after status change

- [x] 2.1 In `PapersView.vue`, modify the `setStatus` function to compute next `selectedIndex` optimistically before re-fetch returns
- [x] 2.2 Implement the logic: if paper will disappear from current filter → keep index; if paper stays → advance to index+1; if was last item → move to index-1
- [x] 2.3 After `fetchPapers()` completes, validate that `selectedIndex` is within bounds of the new list; if not, clamp or recalculate
- [x] 2.4 Ensure detail panel (`openDetailV2`) is called for the newly selected paper after the re-fetch settles

## 3. Contextual action buttons

- [x] 3.1 Add `statusFilter` prop to `PaperDetailPanel.vue` (type: `string`, values: `''`, `'new'`, `'approved'`, `'rejected'`, `'downloaded'`)
- [x] 3.2 Implement button visibility logic in `PaperDetailPanel`: when `statusFilter='approved'` → hide Approve; when `statusFilter='rejected'` → hide Reject; when `statusFilter='downloaded'` → hide Approve; otherwise show both
- [x] 3.3 Pass `statusFilter` from `PapersView.vue` to `PaperDetailPanel` in the template

## 4. Status badges in paper list

- [x] 4.1 Add a status `NTag` column to the paper data table in `PapersView.vue` (before or after the title column), using existing `NTag` from Naive UI
- [x] 4.2 Map status to color: new → `#3b82f6`, approved → `#22c55e`, rejected → `#ef4444`, downloaded → `#8b5cf6`
- [x] 4.3 Keep the existing `opacity: 0.45` for rejected papers as a secondary cue
- [x] 4.4 Add the status badge to the `PaperDetailPanel` header as well (if not already present)

## 5. Background auto-download after approve

- [x] 5.1 In `PapersView.setStatus()`, after setting status to `'approved'`, call `DownloadPaper(paper.id)` as fire-and-forget (no `await`)
- [x] 5.2 Ensure `DownloadPaper` import is available in `PapersView.vue` (via Wails bindings)
- [x] 5.3 Verify that rapid sequential approvals do not cause errors or duplicate downloads

## 6. Replace DownloadModal with non-blocking indicator

- [x] 6.1 Remove the `DownloadModal` trigger from the download banner in `PapersView.vue`; replace with direct `DownloadApproved(pid)` call on banner click
- [x] 6.2 Change banner text from "Download PDF" to show the count and label clearly (e.g., "Download {N} Approved Papers")
- [x] 6.3 Enhance `BackgroundTasksIndicator.vue`: add click handler to open a popover with per-paper download log cards (port the log UI from `DownloadModal.vue`) — extracted the per-paper log into a shared `DownloadLog.vue` component and embedded it in the header popover's download section
- [x] 6.4 Keep the old `DownloadModal.vue` but remove its blocking modal wrapper; optionally repurpose as the detail popover content for the header indicator — `DownloadModal.vue` now delegates to the shared `DownloadLog.vue` (duplicated aggregation logic removed); same log content powers both the modal and the header popover
- [x] 6.5 Remove the confirmation sub-dialog from the download flow — downloads start immediately on button click

## 7. i18n

- [x] 7.1 Add English keys (`en.json`): status badge labels (new/approved/rejected/downloaded), "Download All Approved" banner text, download indicator tooltip text
- [x] 7.2 Add Russian keys (`ru.json`): same set in Russian
- [x] 7.3 Update any existing keys whose wording changed (e.g., download banner, shortcut hint bar)

## 8. Onboarding update

- [x] 8.1 Update driver.js tour steps in `PapersView.vue` (onboarding phase 4): add step highlighting status badges, mention auto-advance behavior in keyboard shortcuts step
- [x] 8.2 Verify `data-onboarding` attributes still point to correct elements after UI changes
- [x] 8.3 Update tour text for any changed button labels or UI elements

## 9. End-to-end validation

- [ ] 9.1 Manual test: on "new" tab, approve paper → verify auto-advance to next, paper disappears, detail panel updates
- [ ] 9.2 Manual test: on "new" tab, reject last paper → verify auto-advance to previous, list correctly shows remaining papers
- [ ] 9.3 Manual test: on "all" tab, approve paper → verify advance to next, paper stays in list with updated badge
- [ ] 9.4 Manual test: switch to "approved" tab → verify only Reject button visible in detail panel
- [ ] 9.5 Manual test: switch to "rejected" tab → verify only Approve button visible
- [ ] 9.6 Manual test: approve a paper → verify header shows download indicator → verify PDF eventually downloaded
- [ ] 9.7 Manual test: click "Download All" → verify downloads start without modal, progress visible in header popover
- [ ] 9.8 Manual test: onboarding tour runs without errors, all steps highlight correct elements
