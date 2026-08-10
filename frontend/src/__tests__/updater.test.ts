import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock Wails bindings and runtime before importing the composable.
vi.mock('../../wailsjs/go/app/App', () => ({
  CheckForUpdates: vi.fn(),
  DownloadUpdate: vi.fn(),
  InstallAndRestart: vi.fn(),
}))

// EventsOn returns an unsubscribe fn; capture the registered callback so tests
// can simulate download-progress events.
let progressCallback: ((data: any) => void) | null = null
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((_event: string, cb: (data: any) => void) => {
    progressCallback = cb
    return () => {}
  }),
}))

import { CheckForUpdates, DownloadUpdate, InstallAndRestart } from '../../wailsjs/go/app/App'
import { useUpdater } from '../composables/useUpdater'

describe('useUpdater', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    progressCallback = null
  })

  // 4.25 — initial state: only "check" is actionable; download/restart hidden.
  it('starts idle with no download or ready state', () => {
    const u = useUpdater()
    expect(u.updateStatus.value).toBe('idle')
    expect(u.checkingUpdate.value).toBe(false)
    expect(u.isDownloading.value).toBe(false)
    expect(u.isUpdateReady.value).toBe(false)
    expect(u.updateError.value).toBe('')
  })

  it('marks an update available when the backend reports one', async () => {
    vi.mocked(CheckForUpdates).mockResolvedValue({
      hasUpdate: true,
      latestVersion: 'v2.0.0',
      assetURL: 'https://example.com/a.zip',
    } as any)

    const u = useUpdater()
    await u.checkUpdates()

    expect(u.updateStatus.value).toBe('available')
    expect(u.latestVersion.value).toBe('v2.0.0')
    expect(u.downloadAssetURL.value).toBe('https://example.com/a.zip')
  })

  it('reports up-to-date when there is no newer release', async () => {
    vi.mocked(CheckForUpdates).mockResolvedValue({
      hasUpdate: false,
      latestVersion: 'v1.0.0',
      assetURL: '',
    } as any)

    const u = useUpdater()
    await u.checkUpdates()

    expect(u.updateStatus.value).toBe('up-to-date')
  })

  it('captures the error message when the check fails', async () => {
    vi.mocked(CheckForUpdates).mockRejectedValue(new Error('network down'))

    const u = useUpdater()
    await expect(u.checkUpdates()).rejects.toThrow('network down')
    expect(u.updateError.value).toContain('network down')
    expect(u.checkingUpdate.value).toBe(false)
  })

  // 4.27 — while downloading, isDownloading is true (buttons disabled).
  it('flags isDownloading during the download and clears it after', async () => {
    let resolveDownload: () => void = () => {}
    vi.mocked(DownloadUpdate).mockReturnValue(
      new Promise<void>((resolve) => { resolveDownload = resolve }),
    )

    const u = useUpdater()
    u.downloadAssetURL.value = 'https://example.com/a.zip'

    const p = u.downloadUpdate()
    expect(u.isDownloading.value).toBe(true)
    expect(u.isUpdateReady.value).toBe(false)

    resolveDownload()
    await p

    // 4.26 — after a successful download the update is ready to install.
    expect(u.isDownloading.value).toBe(false)
    expect(u.isUpdateReady.value).toBe(true)
  })

  it('does nothing when there is no asset URL to download', async () => {
    const u = useUpdater()
    await u.downloadUpdate()
    expect(DownloadUpdate).not.toHaveBeenCalled()
    expect(u.isDownloading.value).toBe(false)
  })

  it('updates the progress percentage from download-progress events', () => {
    const u = useUpdater()
    expect(progressCallback).toBeTypeOf('function')
    progressCallback!({ percentage: 42.6, downloaded: 1, total: 2 })
    expect(u.downloadPercentage.value).toBe(43)
  })

  it('enters restarting state on install and rolls back on failure', async () => {
    vi.mocked(InstallAndRestart).mockRejectedValue(new Error('permission denied'))

    const u = useUpdater()
    await expect(u.installUpdate()).rejects.toThrow('permission denied')
    expect(u.updateStatus.value).toBe('available')
    expect(u.updateError.value).toContain('permission denied')
  })
})
