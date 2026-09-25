package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/qnqatop/papeer/internal/export"
	"github.com/qnqatop/papeer/internal/recsys"
	"github.com/qnqatop/papeer/internal/updater"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/download"
	dlsources "github.com/qnqatop/papeer/internal/download/sources"
	"github.com/qnqatop/papeer/internal/httpclient"
	"github.com/qnqatop/papeer/internal/llm"
	"github.com/qnqatop/papeer/internal/search"
	"github.com/qnqatop/papeer/internal/server"
)

// Version is the build version stamp, overwritten via:
//
//	go build -ldflags "-X github.com/qnqatop/papeer/internal/app.Version=v0.X.Y-abcdef"
//
// Default "dev" identifies an unstamped build (local development).
var Version = "dev"

// AppVersion returns the build version string for display in the UI footer.
func (a *App) AppVersion() string { return Version }

type UpdateInfo = updater.UpdateInfo

func (a *App) CheckForUpdates() (*UpdateInfo, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("updater not initialized")
	}
	return a.updater.CheckForUpdates()
}

func (a *App) DownloadUpdate(assetURL string) error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}

	// Clean up any staging dir from a previous download attempt so repeated
	// downloads don't leak temp directories.
	if prev := a.updater.GetStagingDir(); prev != "" {
		os.RemoveAll(prev)
		a.updater.SetStagingDir("")
	}

	stagingDir, err := os.MkdirTemp("", "papeer-update")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	archivePath, err := a.updater.DownloadUpdate(assetURL, func(downloaded, total int64) {
		percentage := float64(0)
		if total > 0 {
			percentage = float64(downloaded) / float64(total) * 100
		}
		runtime.EventsEmit(a.ctx, "update:download-progress", map[string]interface{}{
			"percentage": percentage,
			"downloaded": downloaded,
			"total":      total,
		})
	})
	if err != nil {
		os.RemoveAll(stagingDir)
		return err
	}
	defer os.Remove(archivePath)

	if _, err := a.updater.ExtractArchive(archivePath, stagingDir); err != nil {
		os.RemoveAll(stagingDir)
		return err
	}

	a.updater.SetStagingDir(stagingDir)
	return nil
}

func (a *App) InstallAndRestart() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	stagingDir := a.updater.GetStagingDir()
	if stagingDir == "" {
		return fmt.Errorf("no downloaded update to install")
	}
	return a.updater.InstallAndRestart(stagingDir)
}

// App is the Wails bindings facade.
// All exported methods are callable from the Vue frontend.
type App struct {
	ctx           context.Context
	db            *db.DB
	updater       *updater.Updater
	cancelMu      sync.Mutex                    // guards cancels, cancelSeq
	cancels       map[uint64]context.CancelFunc // active long-running ops by token
	cancelSeq     uint64
	radarStop     chan struct{}      // close to stop the radar ticker; radarMu guards radar* fields
	radarCancel   context.CancelFunc // cancels an in-flight radar run
	radarStopped  bool               // set by stopRadar; radar must not (re)start
	radarMu       sync.Mutex
	recMu         sync.Mutex // guards RecalculateAIScores
	summaryMu     sync.Mutex // guards summary generation
	updateMu      sync.Mutex // guards pendingUpdate
	pendingUpdate *updater.UpdateInfo
	dlPaperMu     sync.Mutex
	dlPaperSet    map[int64]bool // guards against duplicate single-paper downloads
}

// LLM-related errors.
var (
	ErrSummaryInProgress = errors.New("summary generation already in progress for this paper and model")
	ErrSummaryExists     = errors.New("summary already exists, use RegenerateSummary")
	ErrNoLLMKey          = errors.New("API key not configured for the active LLM profile")
	ErrNoPDF             = errors.New("PDF not downloaded for this paper")
	ErrNoActiveProfile   = errors.New("no active LLM profile; add one in Settings and make it active")
	ErrModelNotInProfile = errors.New("requested model is not part of the active LLM profile")
)

var defaultSummaryPrompt = `You are an academic research assistant. Analyze the provided scientific paper and produce a structured summary in Markdown format:

## Main Idea
One paragraph summarizing the core contribution.

## Method
Brief description of methodology/approach.

## Key Findings
- Bullet points of main results

## Limitations
- Known limitations mentioned by authors

## Relevance
How this paper relates to the broader field. What gap does it fill?

Keep the summary concise (300-500 words). Use the language of the paper.`

// demoProfileName is the name of the LLM profile created during the onboarding
// demo tour (see frontend AppShell.vue demoProfileInitial). Summaries generated
// under this profile use demoSummaryPrompt for a faster, shorter result.
const demoProfileName = "Demo Research"

// demoSummaryPrompt is a compact prompt used during the demo tour: it generates
// faster and fits on screen without scrolling, which reads better in a demo.
var demoSummaryPrompt = `You are a research assistant. Summarize the paper in Markdown, in the language of the paper:

## In one sentence
The core contribution.

## What they did
2–3 sentences on the method.

## Key findings
- 3–4 bullet points

## Why it matters
One sentence on the gap it fills.

Keep it under 200 words.`

// NewApp creates a new App.
func NewApp() *App {
	return &App{
		updater: updater.NewUpdater(Version),
	}
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Startup is called by Wails on application start.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Open database in user's home config dir.
	// migrateDataDir moves ~/.apepa → ~/.papeer (pre-rebrand installs) so beta
	// users keep their databases. Runs once; silently no-ops otherwise.
	home, _ := os.UserHomeDir()
	dbDir := filepath.Join(home, ".papeer")
	if oldDir := filepath.Join(home, ".apepa"); dirExists(oldDir) && !dirExists(dbDir) {
		_ = os.Rename(oldDir, dbDir)
	}
	os.MkdirAll(dbDir, 0o755)

	database, err := db.NewDB(filepath.Join(dbDir, "papeer.db"))
	// Pre-rebrand fallback: reuse existing apepa.db in place if present.
	if err != nil && fileExists(filepath.Join(dbDir, "apepa.db")) {
		database, err = db.NewDB(filepath.Join(dbDir, "apepa.db"))
	}
	if err != nil {
		runtime.LogFatalf(ctx, "Failed to open database: %v", err)
		return
	}
	a.db = database
	server.SetPDFDB(database)

	// One-time migration of the legacy plaintext DeepSeek key into an LLM profile
	// + OS keychain.
	a.backfillLLMProfile()

	// Start radar in background after a delay, if enabled.
	a.safeGo("radar", a.startRadarIfEnabled)

	// Notify frontend if any profile lacks a valid contact email — search and
	// downloads will be blocked until the user updates it.
	a.safeGo("notifyProfilesNeedingEmail", a.notifyProfilesNeedingEmail)

	// Passively check for a newer release (major/minor only) unless disabled.
	a.safeGo("checkForUpdatesPassive", a.checkForUpdatesPassive)
}

// checkForUpdatesPassive runs once on startup: if auto-update checking is
// enabled, it queries GitHub and, when a newer major/minor release exists that
// the user hasn't dismissed, emits "update:available" so the UI can show a
// banner. Patch bumps are intentionally silent (opt-in via the settings panel).
func (a *App) checkForUpdatesPassive() {
	// Small delay so the frontend binds its EventsOn handler before we emit.
	time.Sleep(2 * time.Second)

	if a.updater == nil || a.db == nil {
		return
	}

	settings, err := a.db.GetAllSettings()
	if err != nil {
		return
	}
	if settings["auto_update_check"] == "false" {
		return
	}

	info, err := a.updater.CheckForUpdates()
	if err != nil || info == nil || !info.HasUpdate {
		return
	}
	if info.Severity != "major" && info.Severity != "minor" {
		return
	}
	if settings["update_dismissed_version"] == info.LatestVersion {
		return
	}

	a.updateMu.Lock()
	a.pendingUpdate = info
	a.updateMu.Unlock()

	runtime.EventsEmit(a.ctx, "update:available", info)
}

// PendingUpdate returns the update found by the passive startup check, or nil.
// The frontend calls this on mount to backfill the case where the startup
// event fired before its listener was ready.
func (a *App) PendingUpdate() *updater.UpdateInfo {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.pendingUpdate
}

// notifyProfilesNeedingEmail emits a "profile:needs_email" event for each
// profile with an invalid contact email. The frontend listens and opens an
// edit modal.
func (a *App) notifyProfilesNeedingEmail() {
	// Tiny delay so the frontend has time to bind its EventsOn handler before
	// we emit. Without it the first launch may miss the event.
	time.Sleep(1500 * time.Millisecond)

	profiles, err := a.db.ListProfiles()
	if err != nil {
		return
	}
	for _, p := range profiles {
		if !IsValidEmail(p.Email) {
			runtime.EventsEmit(a.ctx, "profile:needs_email", p)
		}
	}
}

// ProfilesNeedingEmail returns the list of profiles with invalid contact email.
// Frontend calls this on mount to backfill the case where the startup event
// arrived before the listener was ready.
func (a *App) ProfilesNeedingEmail() ([]db.Profile, error) {
	all, err := a.db.ListProfiles()
	if err != nil {
		return nil, err
	}
	var bad []db.Profile
	for _, p := range all {
		if !IsValidEmail(p.Email) {
			bad = append(bad, p)
		}
	}
	return bad, nil
}

// Shutdown is called by Wails on application exit.
func (a *App) Shutdown(_ context.Context) {
	a.stopRadar()
	if a.db != nil {
		a.db.Close()
	}
}

// GetDB returns the underlying database connection.
func (a *App) GetDB() *db.DB {
	return a.db
}

// ── Profiles ─────────────────────────────────────────────

func (a *App) ListProfiles() ([]db.Profile, error) {
	return a.db.ListProfiles()
}

func (a *App) GetProfile(id int64) (*db.Profile, error) {
	return a.db.GetProfile(id)
}

func (a *App) CreateProfile(p db.Profile) (*db.Profile, error) {
	if !IsValidEmail(p.Email) {
		return nil, ErrInvalidEmail
	}
	if p.YearMin == 0 {
		p.YearMin = 2018
	}
	if p.MaxPerQuery == 0 {
		p.MaxPerQuery = 25
	}
	if err := a.db.CreateProfile(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *App) UpdateProfile(p db.Profile) error {
	if !IsValidEmail(p.Email) {
		return ErrInvalidEmail
	}
	return a.db.UpdateProfile(&p)
}

func (a *App) DeleteProfile(id int64) error {
	return a.db.DeleteProfile(id)
}

// ProfileStats returns the number of axes and papers for a profile.
func (a *App) ProfileStats(id int64) (map[string]int, error) {
	axes, papers, err := a.db.ProfileStats(id)
	if err != nil {
		return nil, err
	}
	return map[string]int{"axes": axes, "papers": papers}, nil
}

// SelectDirectory opens a native directory picker dialog.
func (a *App) SelectDirectory(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
	})
}

// ── Settings ────────────────────────────────────────────

func (a *App) GetSettings() (map[string]string, error) {
	return a.db.GetAllSettings()
}

func (a *App) SaveSetting(key, value string) error {
	return a.db.SetSetting(key, value)
}

// TestProxy checks if a proxy URL is reachable by making a test request.
func (a *App) TestProxy(proxyURL string) error {
	client, err := httpclient.NewWithProxy("", proxyURL)
	if err != nil {
		return err
	}
	var result map[string]interface{}
	return client.DoJSON(context.Background(), "https://httpbin.org/get", &result)
}

// ProviderStatus is the result of probing one search provider.
type ProviderStatus struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
	LatencyMs int64  `json:"latency_ms"`
}

// CheckSearchProviders probes the four search-provider endpoints in parallel
// and returns per-provider reachability. The current proxy setting (if any)
// is used so the tester sees the same routing the real search would take.
//
// Timeout is short (15s per provider) so a hung endpoint doesn't block the UI.
func (a *App) CheckSearchProviders() ([]ProviderStatus, error) {
	client, err := a.makeHTTPClient("")
	if err != nil {
		return nil, err
	}

	type probe struct {
		name string
		url  string
		json bool // true → DoJSON, false → DoText (for Atom XML)
	}
	probes := []probe{
		{"semantic_scholar", "https://api.semanticscholar.org/graph/v1/paper/search?query=test&limit=1", true},
		{"openalex", "https://api.openalex.org/works?per-page=1", true},
		{"crossref", "https://api.crossref.org/works?rows=1", true},
		{"arxiv", "http://export.arxiv.org/api/query?search_query=test&max_results=1", false},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results := make([]ProviderStatus, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(i int, p probe) {
			defer wg.Done()
			start := time.Now()
			var err error
			if p.json {
				var sink map[string]interface{}
				err = client.DoJSON(ctx, p.url, &sink)
			} else {
				_, err = client.DoText(ctx, p.url)
			}
			s := ProviderStatus{
				Name:      p.name,
				OK:        err == nil,
				LatencyMs: time.Since(start).Milliseconds(),
			}
			if err != nil {
				s.Error = err.Error()
			}
			results[i] = s
		}(i, p)
	}
	wg.Wait()
	return results, nil
}

// ── Tags ────────────────────────────────────────────────

func (a *App) ListTags(profileID int64) ([]db.Tag, error) {
	return a.db.ListTags(profileID)
}

func (a *App) CreateTag(t db.Tag) (*db.Tag, error) {
	if err := a.db.CreateTag(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (a *App) UpdateTag(t db.Tag) error {
	return a.db.UpdateTag(&t)
}

func (a *App) DeleteTag(id int64) error {
	return a.db.DeleteTag(id)
}

func (a *App) SetPaperTags(paperID int64, tagIDs []int64) error {
	return a.db.SetPaperTags(paperID, tagIDs)
}

func (a *App) ListPaperTags(paperID int64) ([]db.PaperTag, error) {
	return a.db.ListPaperTags(paperID)
}

// ── Axes ─────────────────────────────────────────────────

func (a *App) ListAxes(profileID int64) ([]db.Axis, error) {
	return a.db.ListAxes(profileID)
}

func (a *App) GetAxis(id int64) (*db.Axis, error) {
	return a.db.GetAxis(id)
}

func (a *App) SaveAxis(axis db.Axis) (*db.Axis, error) {
	if err := a.db.SaveAxis(&axis); err != nil {
		return nil, err
	}
	return &axis, nil
}

func (a *App) DeleteAxis(id int64) error {
	return a.db.DeleteAxis(id)
}

func (a *App) ReorderAxes(profileID int64, axisIDs []int64) error {
	return a.db.ReorderAxes(profileID, axisIDs)
}

func (a *App) CountPapersByAxis(profileID int64) (map[int64]int, error) {
	return a.db.CountPapersByAxis(profileID)
}

// ── Papers ───────────────────────────────────────────────

type PaperListResult struct {
	Papers []db.Paper `json:"papers"`
	Total  int        `json:"total"`
}

func (a *App) ListPapers(f db.PaperFilter) (*PaperListResult, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	papers, total, err := a.db.ListPapers(f)
	if err != nil {
		return nil, err
	}
	return &PaperListResult{Papers: papers, Total: total}, nil
}

func (a *App) GetPaper(id int64) (*db.Paper, error) {
	return a.db.GetPaper(id)
}

func (a *App) UpdatePaperStatus(id int64, status string) error {
	err := a.db.UpdatePaperStatus(id, status)

	// Если статус изменился на approved или rejected, запускаем пересчет в фоне
	if err == nil && (status == "approved" || status == "rejected") {
		a.safeGo("RecalculateAIScores", func() {
			// Получаем статью, чтобы узнать её AxisID
			paper, err := a.db.GetPaper(id)
			if err == nil && paper.AxisID != nil {
				a.RecalculateAIScores(paper.ProfileID, *paper.AxisID)
			}
		})
	}
	return err
}

func (a *App) RecalculateAIScores(profileID int64, axisID int64) {
	a.recMu.Lock()
	defer a.recMu.Unlock()

	sendLog := func(msg string) {
		runtime.EventsEmit(a.ctx, "search:progress", search.SearchEvent{
			Type:  "ai_log",
			Axis:  "AI Engine",
			Query: msg,
		})
	}

	result, err := a.ListPapers(db.PaperFilter{
		ProfileID: profileID,
		AxisID:    &axisID,
		Limit:     10000,
	})
	if err != nil {
		sendLog(fmt.Sprintf("❌ Ошибка SQL: %v, axes= %d", err, axisID))
		return
	}
	if len(result.Papers) == 0 {
		sendLog(fmt.Sprintf("⚠️ В оси нет статей (Найдено статей: 0), axes= %d", axisID))
		return
	}

	approvedCount := 0
	for _, p := range result.Papers {
		if p.Status == "approved" {
			approvedCount++
		}
	}

	sendLog(fmt.Sprintf("🔍 Запуск пересчета. Всего статей в оси: %d. Одобрено: %d (нужно минимум 3)", len(result.Papers), approvedCount))

	if approvedCount < 3 {
		sendLog("⚠️ Отмена: сработала защита 'Холодного старта' (мало одобренных).")
		return
	}

	axis, err := a.db.GetAxis(axisID)
	if err != nil {
		sendLog(fmt.Sprintf("❌ Ошибка: не удалось получить данные оси ID %d", axisID))
		return
	}

	var boosts []string
	for _, kw := range axis.Keywords {
		if kw.Type == "boost" {
			boosts = append(boosts, kw.Word)
		}
	}

	scores := recsys.CalculateSmartScores(result.Papers, boosts)

	if len(scores) > 0 {
		sendLog(fmt.Sprintf("✅ Успех: новые оценки назначены для %d статей", len(scores)))
	} else if approvedCount >= 3 {
		sendLog("⚠️ Странно: алгоритм отработал, но вернул 0 оценок (возможно, нет статей со статусом 'new')")
	}

	if err := a.db.UpdateAIScores(scores); err != nil {
		sendLog(fmt.Sprintf("❌ Ошибка записи скоров в БД: %v", err))
		return
	}
	runtime.EventsEmit(a.ctx, "ai_scores_updated")
}

func (a *App) BulkUpdateStatus(ids []int64, status string) (int64, error) {
	return a.db.BulkUpdateStatus(ids, status)
}

func (a *App) SetUserScore(id int64, score int) error {
	return a.db.SetUserScore(id, score)
}

func (a *App) SetPaperNotes(id int64, notes string) error {
	return a.db.SetPaperNotes(id, notes)
}

func (a *App) ApproveByScore(profileID int64, minScore int) (int64, error) {
	return a.db.ApproveByScore(profileID, minScore)
}

// ── Search ───────────────────────────────────────────────

// SearchAxis runs search for a single axis across all providers.
// Emits "search:progress" events to the frontend.
func (a *App) SearchAxis(profileID int64, axisID int64) (int, error) {
	ctx, cancel := context.WithCancel(a.ctx)
	token := a.setCancel(cancel)
	defer func() {
		cancel()
		a.clearCancel(token)
	}()
	return a.searchAxis(ctx, profileID, axisID)
}

// searchAxis runs the search for one axis under ctx (cancelled via
// CancelOperation by the caller that registered it).
func (a *App) searchAxis(ctx context.Context, profileID int64, axisID int64) (int, error) {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return 0, fmt.Errorf("profile %d: %w", profileID, err)
	}
	if !IsValidEmail(profile.Email) {
		return 0, ErrInvalidEmail
	}

	axis, err := a.db.GetAxis(axisID)
	if err != nil {
		return 0, fmt.Errorf("axis %d: %w", axisID, err)
	}

	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		return 0, fmt.Errorf("http client: %w", err)
	}

	providers := []search.Provider{
		search.NewSemanticScholar(client),
		search.NewOpenAlex(client, profile.Email),
		search.NewCrossref(client),
		search.NewArXiv(client),
	}

	onEvent := func(e search.SearchEvent) {
		runtime.EventsEmit(a.ctx, "search:progress", e)
	}

	engine := search.NewEngine(a.db, providers, onEvent)

	queries := make([]string, len(axis.Queries))
	for i, q := range axis.Queries {
		queries[i] = q.Text
	}

	yearMin := profile.YearMin
	if axis.YearMin != nil {
		yearMin = *axis.YearMin
	}
	const maxPerQuery = 10

	count, err := engine.SearchAxis(ctx, search.SearchAxisInput{
		ProfileID:   profileID,
		Axis:        *axis,
		Queries:     queries,
		Keywords:    axis.Keywords,
		YearMin:     yearMin,
		MaxPerQuery: maxPerQuery,
	})
	return count, err
}

// SearchAllAxes runs search for all axes of a profile.
func (a *App) SearchAllAxes(profileID int64) (int, error) {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return 0, fmt.Errorf("profile %d: %w", profileID, err)
	}
	if !IsValidEmail(profile.Email) {
		return 0, ErrInvalidEmail
	}

	axes, err := a.db.ListAxes(profileID)
	if err != nil {
		return 0, err
	}

	// One cancellable context for the whole run: CancelOperation stops the
	// remaining axes instead of only the one currently being searched.
	ctx, cancel := context.WithCancel(a.ctx)
	token := a.setCancel(cancel)
	defer func() {
		cancel()
		a.clearCancel(token)
	}()

	total := 0
	for _, axis := range axes {
		if ctx.Err() != nil {
			break
		}
		count, err := a.searchAxis(ctx, profileID, axis.ID)
		if ctx.Err() != nil {
			total += count // papers saved before the cancel are kept
			break
		}
		if err != nil {
			runtime.EventsEmit(a.ctx, "search:error", map[string]string{
				"axis":  axis.AxisKey,
				"error": err.Error(),
			})
			continue
		}
		total += count
	}

	runtime.EventsEmit(a.ctx, "search:done", map[string]int{"total": total})
	return total, nil
}

// ── Download ─────────────────────────────────────────────

// DownloadApproved downloads PDFs for all approved papers of a profile.
func (a *App) DownloadApproved(profileID int64) error {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return err
	}
	if !IsValidEmail(profile.Email) {
		return ErrInvalidEmail
	}

	if profile.PdfDir == "" {
		return fmt.Errorf("PDF directory not configured for profile %q", profile.Name)
	}

	papers, err := a.db.GetApprovedPapers(profileID)
	if err != nil {
		return err
	}
	if len(papers) == 0 {
		return fmt.Errorf("no approved papers to download")
	}

	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		return err
	}

	// Create the PDF directory up front (synchronously) so a bad path fails
	// loudly here instead of inside the background goroutine where the error
	// would only surface as a progress event.
	if err := os.MkdirAll(profile.PdfDir, 0o755); err != nil {
		return fmt.Errorf("create pdf dir %q: %w", profile.PdfDir, err)
	}

	// Build source chain based on profile config.
	dlSources := buildDownloadSources(profile)

	ctx, cancel := context.WithCancel(a.ctx)
	token := a.setCancel(cancel)

	onEvent := func(e download.DownloadEvent) {
		runtime.EventsEmit(a.ctx, "download:progress", e)
	}

	engine := download.NewEngine(client, a.db, dlSources, profile.PdfDir, 4, onEvent)

	a.safeGo("DownloadApproved", func() {
		defer func() {
			cancel()
			a.clearCancel(token)
			runtime.EventsEmit(a.ctx, "download:done", nil)
		}()
		engine.DownloadAll(ctx, papers, profile.Email)
	})

	return nil
}

func (a *App) DownloadPaper(paperID int64) error {
	a.dlPaperMu.Lock()
	if a.dlPaperSet == nil {
		a.dlPaperSet = make(map[int64]bool)
	}
	if a.dlPaperSet[paperID] {
		a.dlPaperMu.Unlock()
		return nil // already downloading this paper
	}
	a.dlPaperSet[paperID] = true
	a.dlPaperMu.Unlock()

	// Release the guard on every exit path; once the background download
	// starts, it owns the guard and releases it when done.
	started := false
	release := func() {
		a.dlPaperMu.Lock()
		delete(a.dlPaperSet, paperID)
		a.dlPaperMu.Unlock()
	}
	defer func() {
		if !started {
			release()
		}
	}()

	paper, err := a.db.GetPaper(paperID)
	if err != nil {
		return err
	}

	profile, err := a.db.GetProfile(paper.ProfileID)
	if err != nil {
		return err
	}
	if !IsValidEmail(profile.Email) {
		return ErrInvalidEmail
	}
	if profile.PdfDir == "" {
		return fmt.Errorf("PDF directory not configured for profile %q", profile.Name)
	}

	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(profile.PdfDir, 0o755); err != nil {
		return fmt.Errorf("create pdf dir %q: %w", profile.PdfDir, err)
	}

	dlSources := buildDownloadSources(profile)

	ctx, cancel := context.WithCancel(a.ctx)
	token := a.setCancel(cancel)

	onEvent := func(e download.DownloadEvent) {
		runtime.EventsEmit(a.ctx, "download:progress", e)
	}

	engine := download.NewEngine(client, a.db, dlSources, profile.PdfDir, 4, onEvent)

	started = true
	a.safeGo("DownloadPaper", func() {
		defer func() {
			cancel()
			a.clearCancel(token)
			release()
			runtime.EventsEmit(a.ctx, "download:done", nil)
		}()
		engine.DownloadOne(ctx, *paper, profile.Email, 1, 1)
	})

	return nil
}

func (a *App) GetFailedDownloadPaperIDs(profileID int64) (map[int64]bool, error) {
	return a.db.GetFailedDownloadPaperIDs(profileID)
}

func buildDownloadSources(profile *db.Profile) []download.Source {
	// If profile has explicit sources, use those in order.
	// Otherwise use the default chain.
	enabledSet := make(map[string]bool)
	if len(profile.DownloadSources) > 0 {
		for _, s := range profile.DownloadSources {
			enabledSet[s] = true
		}
	}

	allSources := map[string]download.Source{
		"search_report": dlsources.NewSearchReport(),
		"arxiv":         dlsources.NewArXiv(),
		"s2_doi":        dlsources.NewS2ByDOI(),
		"openalex":      dlsources.NewOpenAlex(),
		"unpaywall":     dlsources.NewUnpaywall(),
		"crossref":      dlsources.NewCrossref(),
		"s2_title":      dlsources.NewS2ByTitle(),
	}

	var result []download.Source
	for _, name := range download.DefaultSourceOrder {
		if len(enabledSet) > 0 && !enabledSet[name] {
			continue
		}
		if src, ok := allSources[name]; ok {
			result = append(result, src)
		}
	}
	return result
}

// makeHTTPClient creates an HTTP client, optionally with proxy from settings.
// Also injects per-host API keys (e.g. Semantic Scholar x-api-key) so all
// providers and download sources sharing the client benefit automatically.
func (a *App) makeHTTPClient(email string) (*httpclient.Client, error) {
	proxyURL, _ := a.db.GetSetting("proxy_url")
	var (
		client *httpclient.Client
		err    error
	)
	if proxyURL != "" {
		client, err = httpclient.NewWithProxy(email, proxyURL)
	} else {
		client = httpclient.New(email)
	}
	if err != nil {
		return nil, err
	}

	if s2Key, _ := a.db.GetSetting("semantic_scholar_api_key"); s2Key != "" {
		client.SetHostHeader("api.semanticscholar.org", "x-api-key", s2Key)
		// With an API key, Semantic Scholar's documented rate limit is ~1
		// req/s (vs ~100 req/5min ≈ 0.33 req/s unauthenticated). Without this
		// the limiter kept throttling at the slow default even when a key
		// was configured — the exact bug the batch citation fetch fixes.
		client.SetHostRateLimit("api.semanticscholar.org", 1)
	}

	return client, nil
}

// CancelOperation cancels every active long-running operation (search,
// download, citation fetch, review draft, radar run).
func (a *App) CancelOperation() {
	a.cancelMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(a.cancels))
	for _, c := range a.cancels {
		cancels = append(cancels, c)
	}
	a.cancelMu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// setCancel registers the cancel func of a long-running operation and returns
// a token for clearCancel. Several operations may be active at once.
func (a *App) setCancel(c context.CancelFunc) uint64 {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	if a.cancels == nil {
		a.cancels = make(map[uint64]context.CancelFunc)
	}
	a.cancelSeq++
	a.cancels[a.cancelSeq] = c
	return a.cancelSeq
}

// clearCancel unregisters the operation identified by token once it finishes;
// other operations' cancel funcs are left intact.
func (a *App) clearCancel(token uint64) {
	a.cancelMu.Lock()
	delete(a.cancels, token)
	a.cancelMu.Unlock()
}

// logPanic reports a recovered panic; a variable so tests without a Wails
// runtime context can replace it.
var logPanic = func(ctx context.Context, format string, args ...interface{}) {
	runtime.LogErrorf(ctx, format, args...)
}

// safeGo runs fn in a new goroutine. A panic in fn is recovered and logged
// instead of crashing the whole app (an unrecovered panic in any goroutine
// terminates the process). Deferred calls inside fn still run.
func (a *App) safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logPanic(a.ctx, "panic in %s: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// ── Radar / Monitoring ───────────────────────────────────

const radarTagColor = "#f97316" // orange

// RunRadar runs a lightweight monitoring search across all axes of a profile.
// It searches with a small limit (3) and recent year filter, then tags
// genuinely new papers with the "monitoring" tag. Returns the count of new papers.
func (a *App) RunRadar(profileID int64) (int, error) {
	return a.runRadar(a.ctx, profileID)
}

// runRadar is RunRadar under a parent context, so the scheduled radar can be
// interrupted by stopRadar on shutdown.
func (a *App) runRadar(parent context.Context, profileID int64) (int, error) {
	profile, err := a.db.GetProfile(profileID)
	if err != nil {
		return 0, fmt.Errorf("profile %d: %w", profileID, err)
	}
	if !IsValidEmail(profile.Email) {
		return 0, ErrInvalidEmail
	}

	axes, err := a.db.ListAxes(profileID)
	if err != nil {
		return 0, fmt.Errorf("list axes: %w", err)
	}

	startTime := time.Now()
	client, err := a.makeHTTPClient(profile.Email)
	if err != nil {
		return 0, fmt.Errorf("http client: %w", err)
	}

	providers := []search.Provider{
		search.NewSemanticScholar(client),
		search.NewOpenAlex(client, profile.Email),
		search.NewCrossref(client),
		search.NewArXiv(client),
	}

	radarLimit := 3
	// Radar looks at papers from the current and previous year.
	radarYearMin := time.Now().Year() - 1
	if profile.YearMin > radarYearMin {
		radarYearMin = profile.YearMin
	}

	// Radar uses the same event channel as regular search so progress
	// appears in the Search Log view.
	onEvent := func(e search.SearchEvent) {
		runtime.EventsEmit(a.ctx, "search:progress", e)
	}

	engine := search.NewEngine(a.db, providers, onEvent)

	ctx, cancel := context.WithCancel(parent)
	token := a.setCancel(cancel)
	defer func() {
		cancel()
		a.clearCancel(token)
	}()

	totalProcessed := 0
	for _, axis := range axes {
		if ctx.Err() != nil {
			break // cancelled: still tag what was found so far
		}
		if len(axis.Queries) == 0 {
			continue
		}

		yearMin := radarYearMin
		if axis.YearMin != nil && *axis.YearMin > yearMin {
			yearMin = *axis.YearMin
		}

		queries := make([]string, len(axis.Queries))
		for i, q := range axis.Queries {
			queries[i] = q.Text
		}

		count, err := engine.SearchAxis(ctx, search.SearchAxisInput{
			ProfileID:   profileID,
			Axis:        axis,
			Queries:     queries,
			Keywords:    axis.Keywords,
			YearMin:     yearMin,
			MaxPerQuery: radarLimit,
		})
		if err != nil {
			runtime.LogErrorf(ctx, "radar: axis %s: %v", axis.AxisKey, err)
			continue
		}
		totalProcessed += count

		// Record when this axis was last checked.
		a.db.SetAxisRadarRun(axis.ID, time.Now())
	}

	// Find genuinely new papers (INSERT, not UPDATE during merge).
	newIDs, err := a.db.GetPaperIDsAfterTime(profileID, startTime)
	if err != nil {
		return 0, fmt.Errorf("find new papers: %w", err)
	}

	// Tag new papers with the "monitoring" tag.
	if len(newIDs) > 0 {
		tagID, err := a.db.EnsureTag(profileID, "monitoring", radarTagColor)
		if err != nil {
			runtime.LogErrorf(ctx, "radar: ensure tag: %v", err)
		} else {
			for _, paperID := range newIDs {
				// Add the tag without a read-modify-write of the paper's
				// tags, so a failed read can't wipe its existing tags.
				if err := a.db.AddTagToPaper(tagID, paperID); err != nil {
					runtime.LogErrorf(ctx, "radar: tag paper %d: %v", paperID, err)
				}
			}
		}
	}

	// Emit event.
	runtime.EventsEmit(a.ctx, "radar:done", map[string]interface{}{
		"new_papers": len(newIDs),
		"profile_id": profileID,
		"processed":  totalProcessed,
	})

	return len(newIDs), nil
}

// radarStartDelay lets the UI settle before the first radar run; a variable
// so tests don't have to wait.
var radarStartDelay = 10 * time.Second

// initRadar registers the stop channel and cancellable context of the radar
// scheduler. It returns ok=false if stopRadar already ran (app shutting down).
// radarMu is held only here and in stopRadar — never across a radar run —
// so stopRadar can't block behind the scheduler.
func (a *App) initRadar() (stop <-chan struct{}, ctx context.Context, ok bool) {
	a.radarMu.Lock()
	defer a.radarMu.Unlock()
	if a.radarStopped || a.radarStop != nil {
		return nil, nil, false
	}
	ch := make(chan struct{})
	ctx, cancel := context.WithCancel(a.ctx)
	a.radarStop = ch
	a.radarCancel = cancel
	return ch, ctx, true
}

// startRadarIfEnabled reads settings and starts the radar with the configured frequency.
func (a *App) startRadarIfEnabled() {
	stop, ctx, ok := a.initRadar()
	if !ok {
		return
	}

	select {
	case <-time.After(radarStartDelay): // let UI settle
	case <-stop:
		return
	}

	enabled, _ := a.db.GetSetting("enable_radar")
	if enabled != "true" {
		return
	}

	runAllProfiles := func() {
		profiles, err := a.db.ListProfiles()
		if err != nil {
			return
		}
		for _, p := range profiles {
			select {
			case <-stop:
				return
			default:
			}
			count, err := a.runRadar(ctx, p.ID)
			if err != nil {
				runtime.LogErrorf(a.ctx, "radar: profile %q: %v", p.Name, err)
			} else if count > 0 {
				runtime.LogInfof(a.ctx, "radar: profile %q: %d new papers", p.Name, count)
			}
		}
	}

	// Run immediately once, then schedule periodic runs.
	runAllProfiles()

	freq, _ := a.db.GetSetting("radar_frequency")
	if freq == "" || freq == "startup" {
		return // one-shot only
	}

	duration := parseRadarFrequency(freq)
	if duration <= 0 {
		return
	}

	radarLoop(stop, duration, runAllProfiles)
}

// radarLoop calls run every interval until stop is closed.
func radarLoop(stop <-chan struct{}, interval time.Duration, run func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			run()
		case <-stop:
			return
		}
	}
}

// stopRadar stops the radar scheduler and cancels an in-flight radar run.
// It never waits for the scheduler, so it can't hang app shutdown.
func (a *App) stopRadar() {
	a.radarMu.Lock()
	defer a.radarMu.Unlock()
	a.radarStopped = true
	if a.radarStop != nil {
		close(a.radarStop)
		a.radarStop = nil
	}
	if a.radarCancel != nil {
		a.radarCancel()
		a.radarCancel = nil
	}
}

// parseRadarFrequency converts a frequency string to a duration.
func parseRadarFrequency(freq string) time.Duration {
	switch freq {
	case "3h":
		return 3 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "24h":
		return 24 * time.Hour
	default:
		return 0
	}
}

func (a *App) AxesWithPapers(profileID int64) ([]db.Axis, error) {
	return a.db.AxesWithPapers(profileID)
}

// ── Statistics ───────────────────────────────────────────

type StatsResult struct {
	PapersByStatus    map[string]int    `json:"papers_by_status"`
	ScoreDistribution map[int]int       `json:"score_distribution"`
	YearDistribution  map[int]int       `json:"year_distribution"`
	DownloadStats     *db.DownloadStats `json:"download_stats"`
	// TopAuthors and CitationBuckets are scoped by the same optional axisID
	// filter as the rest of GetStats (via the paper_axes M2M table).
	TopAuthors      []db.AuthorCount    `json:"top_authors"`
	CitationBuckets []db.CitationBucket `json:"citation_buckets"`
}

// GetStats returns dashboard statistics for a profile, optionally scoped to
// one axis. The axis filter goes through the paper_axes many-to-many table
// so it reflects every axis a paper was actually found in, not just the
// (legacy, single-valued) papers.axis_id column.
func (a *App) GetStats(profileID int64, axisID *int64) (*StatsResult, error) {
	byStatus, err := a.db.PapersByStatus(profileID, axisID)
	if err != nil {
		return nil, err
	}

	scoreDist, err := a.db.ScoreDistribution(profileID, axisID)
	if err != nil {
		return nil, err
	}

	yearDist, err := a.db.YearDistribution(profileID, axisID)
	if err != nil {
		return nil, err
	}

	dlStats, err := a.db.GetDownloadStats(profileID)
	if err != nil {
		return nil, err
	}

	topAuthors, err := a.db.TopAuthors(profileID, axisID, 10)
	if err != nil {
		return nil, err
	}

	citationBuckets, err := a.db.CitationCountBuckets(profileID, axisID)
	if err != nil {
		return nil, err
	}

	return &StatsResult{
		PapersByStatus:    byStatus,
		ScoreDistribution: scoreDist,
		YearDistribution:  yearDist,
		DownloadStats:     dlStats,
		TopAuthors:        topAuthors,
		CitationBuckets:   citationBuckets,
	}, nil
}

// GetPDFAvailability returns the share of approved/downloaded papers that
// have a successfully downloaded PDF on disk. Backs the "PDF availability %"
// metric (moved into Analysis from the old Download Stats widget).
func (a *App) GetPDFAvailability(profileID int64) (*db.PDFAvailability, error) {
	return a.db.GetPDFAvailability(profileID)
}

// ImportYAML opens a file dialog and imports axes from a YAML file into the profile.
func (a *App) ImportYAML(profileID int64) (int, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import YAML axes config",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil {
		return 0, err
	}
	if path == "" {
		return 0, nil // cancelled
	}

	return a.importYAMLFile(profileID, path)
}

// importYAMLFile imports axes from a YAML file at path. All axes are saved in
// one transaction after the profile's existing axes: a failure (e.g. a
// duplicate axis_key) imports nothing.
func (a *App) importYAMLFile(profileID int64, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > export.MaxYAMLImportSize {
		return 0, export.ErrYAMLTooLarge
	}

	// ImportAxesFromYAML also caps what it reads, in case the file grows.
	axes, err := export.ImportAxesFromYAML(f, profileID)
	if err != nil {
		return 0, err
	}

	if err := a.db.AppendAxes(profileID, axes); err != nil {
		return 0, err
	}
	return len(axes), nil
}

// ExportBibTeX exports approved/downloaded papers as BibTeX.
func (a *App) ExportBibTeX(profileID int64) (string, error) {
	papers, err := a.db.GetApprovedPapers(profileID)
	if err != nil {
		return "", err
	}
	downloaded, _, err := a.db.ListPapers(db.PaperFilter{
		ProfileID: profileID,
		Status:    "downloaded",
		Limit:     10000,
	})
	if err != nil {
		return "", err
	}
	papers = append(papers, downloaded...)
	if len(papers) == 0 {
		return "", fmt.Errorf("no approved or downloaded papers to export")
	}

	var buf strings.Builder
	if err := export.ExportBibTeX(papers, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ExportCSV exports approved/downloaded papers as CSV.
func (a *App) ExportCSV(profileID int64) (string, error) {
	papers, err := a.db.GetApprovedPapers(profileID)
	if err != nil {
		return "", err
	}
	downloaded, _, err := a.db.ListPapers(db.PaperFilter{
		ProfileID: profileID,
		Status:    "downloaded",
		Limit:     10000,
	})
	if err != nil {
		return "", err
	}
	papers = append(papers, downloaded...)
	if len(papers) == 0 {
		return "", fmt.Errorf("no approved or downloaded papers to export")
	}

	var buf strings.Builder
	if err := export.ExportCSV(papers, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ExportFailedDownloads builds a structured text report for every approved
// paper that has no successful download attempt, formatted to match the
// `failed_downloads.txt` template in docs/TEST_FLOW.md so the tester can hand
// it back to the developer for reproduction.
//
// One block per paper, separated by a blank line:
//
//	Title: <title>
//	DOI: <doi or "—">
//	URL издателя: <doi.org link / pdf_url / "—">
//	Свободно ли доступен PDF на сайте издателя: ___
//	Источник, на котором затерялась: <last fail source: reason>
//	Был ли запущен Chrome-fallback: YES/NO
func (a *App) ExportFailedDownloads(profileID int64) (string, error) {
	approved, err := a.db.GetApprovedPapers(profileID)
	if err != nil {
		return "", err
	}
	if len(approved) == 0 {
		return "", fmt.Errorf("no approved papers")
	}

	var buf strings.Builder
	failedCount := 0

	for _, p := range approved {
		downloads, err := a.db.ListDownloads(p.ID)
		if err != nil {
			continue
		}

		hasOK := false
		var lastFail *db.Download
		chromeTried := false
		for i := range downloads {
			dl := downloads[i]
			if dl.Status == "ok" {
				hasOK = true
				break
			}
			if dl.Status == "fail" && lastFail == nil {
				lastFail = &dl
			}
			if strings.Contains(strings.ToLower(dl.Reason), "chrome") ||
				strings.Contains(strings.ToLower(dl.Reason), "headless") {
				chromeTried = true
			}
		}
		if hasOK {
			continue
		}

		failedCount++

		doi := "—"
		if p.DOI != nil && *p.DOI != "" {
			doi = *p.DOI
		}
		pubURL := "—"
		if p.DOI != nil && *p.DOI != "" {
			pubURL = "https://doi.org/" + *p.DOI
		} else if p.PdfURL != nil && *p.PdfURL != "" {
			pubURL = *p.PdfURL
		}
		lastSource := "—"
		if lastFail != nil {
			reason := lastFail.Reason
			if reason == "" {
				reason = "no reason recorded"
			}
			lastSource = fmt.Sprintf("%s: %s", lastFail.Source, reason)
		}
		chromeFlag := "NO"
		if chromeTried {
			chromeFlag = "YES"
		}

		fmt.Fprintf(&buf, "Title: %s\n", p.Title)
		fmt.Fprintf(&buf, "DOI: %s\n", doi)
		fmt.Fprintf(&buf, "URL издателя: %s\n", pubURL)
		fmt.Fprintf(&buf, "Свободно ли доступен PDF на сайте издателя (проверить вручную в браузере без VPN): ___\n")
		fmt.Fprintf(&buf, "Источник, на котором затерялась: %s\n", lastSource)
		fmt.Fprintf(&buf, "Был ли запущен Chrome-fallback (есть ли в логе chrome / headless): %s\n", chromeFlag)
		fmt.Fprintln(&buf)
	}

	if failedCount == 0 {
		return "", fmt.Errorf("no failed downloads for profile %d", profileID)
	}
	return buf.String(), nil
}

// SaveExportFile opens a save dialog and writes content to the chosen path.
func (a *App) SaveExportFile(defaultName string, content string) error {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save Export",
		DefaultFilename: defaultName,
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil // cancelled
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ExportAxisYAML открывает диалог сохранения и экспортирует ось в YAML
func (a *App) ExportAxisYAML(axisID int64) error {
	axis, err := a.db.GetAxis(axisID)
	if err != nil {
		return err
	}

	data, err := export.ExportAxisToYAML(axis)
	if err != nil {
		return fmt.Errorf("generate yaml: %w", err)
	}

	defaultName := axis.AxisKey + ".yaml"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Axis to YAML",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
		},
	})

	if err != nil {
		return err
	}
	if path == "" {
		return nil // Отменено пользователем
	}

	return os.WriteFile(path, data, 0o644)
}

// ExportAllAxesYAML экспортирует все оси профиля в один YAML-файл.
func (a *App) ExportAllAxesYAML(profileID int64) error {
	axes, err := a.db.ListAxes(profileID)
	if err != nil {
		return err
	}
	if len(axes) == 0 {
		return fmt.Errorf("no axes to export")
	}

	data, err := export.ExportAxesToYAML(axes)
	if err != nil {
		return fmt.Errorf("generate yaml: %w", err)
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export All Axes to YAML",
		DefaultFilename: "axes.yaml",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML Files", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

// ── LLM Summary ─────────────────────────────────────────

// backfillLLMProfile performs a one-time migration: if a legacy plaintext
// DeepSeek key exists in app_settings and no LLM profiles exist yet, create a
// DeepSeek profile, move the key into the OS keychain, activate it, and wipe the
// plaintext key. The obsolete Gemini key (if any) is dropped, not migrated.
// If the keychain is unavailable the plaintext key is preserved (so it is not
// lost) and the migration is retried on a later launch.
func (a *App) backfillLLMProfile() {
	// Gemini is no longer supported; drop its key regardless.
	if v, _ := a.db.GetSetting("llm_gemini_api_key"); v != "" {
		_ = a.db.SetSetting("llm_gemini_api_key", "")
	}

	profiles, err := a.db.ListLLMProfiles()
	if err != nil || len(profiles) > 0 {
		return // already migrated or table not ready
	}

	deepseekKey, _ := a.db.GetSetting("llm_deepseek_api_key")
	if deepseekKey == "" {
		return // nothing to migrate
	}

	p := &db.LLMProfile{
		Name:         "DeepSeek",
		BaseURL:      llm.DeepSeekBaseURL,
		Models:       llm.DeepSeekModels,
		DefaultModel: llm.DeepSeekDefaultModel,
		Temperature:  0.2,
	}
	if err := a.db.CreateLLMProfile(p); err != nil {
		return
	}

	// Move the key into the keychain. If the keychain is unavailable, keep the
	// plaintext key so it is not lost — migration retries next launch (the
	// profile row is deleted so the "no profiles" precondition holds again).
	if err := llm.SetKey(p.ID, deepseekKey); err != nil {
		runtime.LogWarningf(a.ctx, "LLM backfill: keychain unavailable, keeping plaintext key for retry: %v", err)
		_ = a.db.DeleteLLMProfile(p.ID)
		return
	}

	if err := a.db.SetActiveLLMProfile(p.ID); err != nil {
		runtime.LogWarningf(a.ctx, "LLM backfill: failed to activate profile: %v", err)
	}

	// Key is safely in the keychain — wipe the plaintext copy.
	_ = a.db.SetSetting("llm_deepseek_api_key", "")
}

// LLMProfileView is the profile shape sent to the frontend: it carries a
// masked key instead of the raw secret, which never leaves the backend.
type LLMProfileView struct {
	db.LLMProfile
	KeyMask string `json:"key_mask"` // e.g. "sk-…a1b2", empty if no key
	HasKey  bool   `json:"has_key"`
}

func (a *App) toProfileView(p db.LLMProfile) LLMProfileView {
	key, _ := llm.GetKey(p.ID)
	return LLMProfileView{
		LLMProfile: p,
		KeyMask:    llm.MaskKey(key),
		HasKey:     key != "",
	}
}

// ListLLMProfiles returns all LLM profiles with masked keys (never the raw key).
func (a *App) ListLLMProfiles() ([]LLMProfileView, error) {
	profiles, err := a.db.ListLLMProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]LLMProfileView, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, a.toProfileView(p))
	}
	return out, nil
}

// CreateLLMProfile creates a profile and stores its API key in the OS keychain.
// The is_active flag on the input decides whether the new profile becomes active.
// If the keychain is unavailable the profile is still created (key can be set
// later) and a soft error is returned.
func (a *App) CreateLLMProfile(profile db.LLMProfile, apiKey string) (*LLMProfileView, error) {
	if err := validateLLMBaseURL(profile.BaseURL); err != nil {
		return nil, err
	}
	wantActive := profile.IsActive
	profile.IsActive = false // activation is handled transactionally below
	if err := a.db.CreateLLMProfile(&profile); err != nil {
		return nil, err
	}

	var keyErr error
	if apiKey != "" {
		if err := llm.SetKey(profile.ID, apiKey); err != nil {
			keyErr = err // soft: profile exists, key just wasn't saved
		}
	}
	if wantActive {
		if err := a.db.SetActiveLLMProfile(profile.ID); err != nil {
			return nil, err
		}
		profile.IsActive = true
	}

	saved, err := a.db.GetLLMProfile(profile.ID)
	if err != nil {
		return nil, err
	}
	view := a.toProfileView(*saved)
	return &view, keyErr
}

// UpdateLLMProfile updates a profile. An empty apiKey means "leave the stored
// key unchanged"; a non-empty apiKey replaces it. If the input is_active is set
// the profile is (re)activated.
//
// If the base URL's origin (scheme+host+port) changes while a key is stored,
// a new apiKey is required: otherwise the stored key would be sent to a host
// the user never entered it for.
func (a *App) UpdateLLMProfile(profile db.LLMProfile, apiKey string) (*LLMProfileView, error) {
	if err := validateLLMBaseURL(profile.BaseURL); err != nil {
		return nil, err
	}
	if apiKey == "" {
		old, err := a.db.GetLLMProfile(profile.ID)
		if err != nil {
			return nil, err
		}
		if llmOrigin(old.BaseURL) != llmOrigin(profile.BaseURL) {
			// A keychain error means we can't tell whether a key is stored:
			// refuse rather than risk re-targeting it.
			if key, err := llm.GetKey(profile.ID); err != nil || key != "" {
				return nil, ErrLLMKeyReentry
			}
		}
	}
	if err := a.db.UpdateLLMProfile(&profile); err != nil {
		return nil, err
	}

	var keyErr error
	if apiKey != "" {
		if err := llm.SetKey(profile.ID, apiKey); err != nil {
			keyErr = err
		}
	}
	if profile.IsActive {
		if err := a.db.SetActiveLLMProfile(profile.ID); err != nil {
			return nil, err
		}
	}

	saved, err := a.db.GetLLMProfile(profile.ID)
	if err != nil {
		return nil, err
	}
	view := a.toProfileView(*saved)
	return &view, keyErr
}

// DeleteLLMProfile removes a profile and its keychain entry.
func (a *App) DeleteLLMProfile(id int64) error {
	if err := a.db.DeleteLLMProfile(id); err != nil {
		return err
	}
	// Best-effort key cleanup; a stale key without a profile is harmless.
	_ = llm.DeleteKey(id)
	return nil
}

// SetActiveLLMProfile makes the given profile the active one.
func (a *App) SetActiveLLMProfile(id int64) error {
	return a.db.SetActiveLLMProfile(id)
}

// TestLLMProfile runs a short real ping against a stored profile's endpoint.
func (a *App) TestLLMProfile(id int64) error {
	p, err := a.db.GetLLMProfile(id)
	if err != nil {
		return err
	}
	// Profiles saved before base URL validation existed may still be http.
	if err := validateLLMBaseURL(p.BaseURL); err != nil {
		return err
	}
	key, err := llm.GetKey(id)
	if err != nil {
		return err
	}
	return a.pingLLM(key, p.BaseURL, p.DefaultModel, p.Temperature)
}

// TestLLMProfileDraft tests an unsaved profile draft (form "Test connection").
func (a *App) TestLLMProfileDraft(baseURL, apiKey, model string) error {
	if err := validateLLMBaseURL(baseURL); err != nil {
		return err
	}
	return a.pingLLM(apiKey, baseURL, model, 0.2)
}

func (a *App) pingLLM(key, baseURL, model string, temperature float64) error {
	if model == "" {
		return fmt.Errorf("no model specified for connection test")
	}
	cli := llm.NewOpenAIClient(key, baseURL, model, float32(temperature))
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	if _, err := cli.Complete(ctx, "You are a health check.", "ping"); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}

// resolveLLM loads the active profile, validates the requested model, and
// builds a client. An empty model selects the profile's DefaultModel.
func (a *App) resolveLLM(model string) (client *llm.OpenAIClient, profile *db.LLMProfile, chosenModel string, err error) {
	p, err := a.db.GetActiveLLMProfile()
	if err != nil {
		return nil, nil, "", err
	}
	if p == nil {
		return nil, nil, "", ErrNoActiveProfile
	}

	chosenModel = strings.TrimSpace(model)
	if chosenModel == "" {
		chosenModel = p.DefaultModel
	} else if !containsString(p.Models, chosenModel) {
		return nil, nil, "", fmt.Errorf("%w: %q is not in profile %q", ErrModelNotInProfile, chosenModel, p.Name)
	}
	if chosenModel == "" {
		return nil, nil, "", fmt.Errorf("no model available in active profile")
	}

	key, err := llm.GetKey(p.ID)
	if err != nil {
		return nil, nil, "", err
	}
	if key == "" {
		return nil, nil, "", ErrNoLLMKey
	}

	return llm.NewOpenAIClient(key, p.BaseURL, chosenModel, float32(p.Temperature)), p, chosenModel, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GenerateSummary starts summary generation for a paper using the active LLM
// profile. An empty model selects the profile's DefaultModel; otherwise model
// must belong to the profile's model list.
func (a *App) GenerateSummary(paperID int64, model string) (*db.Summary, error) {
	client, profile, chosenModel, err := a.resolveLLM(model)
	if err != nil {
		return nil, err
	}

	a.summaryMu.Lock()

	// Check if already generating or done
	status, err := a.db.HasActiveSummary(paperID, chosenModel)
	if err != nil {
		a.summaryMu.Unlock()
		return nil, err
	}
	if status == "generating" {
		a.summaryMu.Unlock()
		return nil, ErrSummaryInProgress
	}
	if status == "done" {
		a.summaryMu.Unlock()
		return nil, ErrSummaryExists
	}

	// Get PDF path
	pdfPath, err := a.db.GetPaperPDFPath(paperID)
	if err != nil {
		a.summaryMu.Unlock()
		return nil, ErrNoPDF
	}

	prompt := a.summaryPrompt(profile)

	// Create record with "generating" status. Upsert: a previous attempt may
	// have left an 'error' (or 'pending') row for this paper+model, and
	// UNIQUE(paper_id, model) would reject a plain insert.
	s := &db.Summary{
		PaperID:  paperID,
		Provider: profile.Name,
		Model:    chosenModel,
		Status:   "generating",
	}
	if err := a.db.UpsertSummary(s); err != nil {
		a.summaryMu.Unlock()
		return nil, err
	}

	a.summaryMu.Unlock()

	return a.runSummary(s, client, pdfPath, prompt, chosenModel)
}

// RegenerateSummary regenerates a summary for a paper+model using the active profile.
func (a *App) RegenerateSummary(paperID int64, model string) (*db.Summary, error) {
	client, profile, chosenModel, err := a.resolveLLM(model)
	if err != nil {
		return nil, err
	}

	a.summaryMu.Lock()

	status, err := a.db.HasActiveSummary(paperID, chosenModel)
	if err != nil {
		a.summaryMu.Unlock()
		return nil, err
	}
	if status == "generating" {
		a.summaryMu.Unlock()
		return nil, ErrSummaryInProgress
	}

	// Check prerequisites before creating the "generating" record
	pdfPath, err := a.db.GetPaperPDFPath(paperID)
	if err != nil {
		a.summaryMu.Unlock()
		return nil, ErrNoPDF
	}

	// Now safe to create the "generating" record
	s := &db.Summary{
		PaperID:  paperID,
		Provider: profile.Name,
		Model:    chosenModel,
		Status:   "generating",
		Content:  "",
	}
	if err := a.db.UpsertSummary(s); err != nil {
		a.summaryMu.Unlock()
		return nil, err
	}
	a.summaryMu.Unlock()

	prompt := a.summaryPrompt(profile)
	return a.runSummary(s, client, pdfPath, prompt, chosenModel)
}

// summaryPrompt returns the summary system prompt for the given profile. A
// user-configured prompt (llm_summary_prompt setting) always wins; otherwise the
// demo profile gets the compact demoSummaryPrompt and everyone else the default.
func (a *App) summaryPrompt(profile *db.LLMProfile) string {
	if prompt, _ := a.db.GetSetting("llm_summary_prompt"); prompt != "" {
		return prompt
	}
	if profile != nil && profile.Name == demoProfileName {
		return demoSummaryPrompt
	}
	return defaultSummaryPrompt
}

// summaryTimeout bounds a single summary LLM call.
const summaryTimeout = 5 * time.Minute

// runSummary extracts the PDF text, calls the LLM, and finalizes the summary
// record. Runs outside the summary lock.
func (a *App) runSummary(s *db.Summary, client llm.Client, pdfPath, prompt, model string) (out *db.Summary, err error) {
	// A panic (e.g. in PDF parsing) must not leave the row in 'generating'.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("summary generation failed: %v", r)
			out = nil
			a.db.UpdateSummary(&db.Summary{ID: s.ID, Status: "error", ErrorMsg: err.Error()})
		}
	}()

	text, err := llm.ExtractText(pdfPath)
	if err != nil {
		a.db.UpdateSummary(&db.Summary{ID: s.ID, Status: "error", ErrorMsg: err.Error()})
		return nil, err
	}

	// Bound the LLM call so a hung endpoint can't keep the summary in
	// 'generating' indefinitely.
	ctx, cancel := context.WithTimeout(a.ctx, summaryTimeout)
	defer cancel()

	// Summary is markdown, so use Complete (temperature from profile), not
	// CompleteDeterministic (which imposes response_format json_object).
	result, err := client.Complete(ctx, prompt, text)
	if err != nil {
		a.db.UpdateSummary(&db.Summary{ID: s.ID, Status: "error", ErrorMsg: err.Error()})
		return nil, err
	}

	s.Content = result.Content
	s.TokensIn = result.Usage.PromptTokens
	s.TokensOut = result.Usage.CompletionTokens
	s.Status = "done"
	if err := a.db.UpdateSummary(s); err != nil {
		return nil, err
	}

	// Reload to get updated timestamps
	updated, _ := a.db.GetSummary(s.PaperID, model)
	if updated != nil {
		return updated, nil
	}
	return s, nil
}

// GetSummaries returns all summaries for a paper.
func (a *App) GetSummaries(paperID int64) ([]db.Summary, error) {
	return a.db.GetSummariesByPaper(paperID)
}

// ListSummaries returns all summaries for a profile.
func (a *App) ListSummaries(profileID int64) ([]db.SummaryWithPaper, error) {
	return a.db.ListSummariesByProfile(profileID)
}

// DeleteSummary deletes a summary.
func (a *App) DeleteSummary(id int64) error {
	return a.db.DeleteSummary(id)
}

// GetPaperPDFPath returns the path to the downloaded PDF for a paper.
func (a *App) GetPaperPDFPath(paperID int64) (string, error) {
	return a.db.GetPaperPDFPath(paperID)
}

// OpenSystemPDF opens the downloaded PDF in the system default viewer via the
// platform opener (xdg-open / open / rundll32, see app_open_*.go).
// runtime.BrowserOpenURL can't be used: Wails rejects file:// URLs silently.
func (a *App) OpenSystemPDF(paperID int64) error {
	path, err := a.db.GetPaperPDFPath(paperID)
	if err != nil {
		return err
	}
	if err := validatePDFPath(path); err != nil {
		return err
	}
	cmd := openFileCommand(path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open pdf: %w", err)
	}
	// Reap the opener process; its exit status is irrelevant.
	go func() { _ = cmd.Wait() }()
	return nil
}

// validatePDFPath checks that path is an absolute path to an existing .pdf
// file before it is handed to the OS opener.
func validatePDFPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("pdf path is not absolute: %q", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".pdf") {
		return fmt.Errorf("not a pdf file: %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("pdf file not found: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pdf path is not a regular file: %q", path)
	}
	return nil
}

// GetDownloadedPapers returns papers with status='downloaded' that have a
// local PDF file on disk.
func (a *App) GetDownloadedPapers(profileID int64) ([]db.Paper, error) {
	return a.db.GetDownloadedPapers(profileID)
}

// GetPaperPDFData читает PDF-файл с диска и отдаёт его в формате Base64.
func (a *App) GetPaperPDFData(paperID int64) (string, error) {
	path, err := a.db.GetPaperPDFPath(paperID)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}
