import { ref, getCurrentInstance, onUnmounted } from 'vue'
import { CheckForUpdates, DownloadUpdate, InstallAndRestart } from '../../wailsjs/go/app/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export type UpdateStatus = 'idle' | 'up-to-date' | 'available' | 'restarting'

/**
 * Drives the "Updates" panel: check → download (with live progress) →
 * install & restart. Holds all reactive state so the component stays a thin
 * view, and cleans up its Wails event listener on unmount.
 */
export function useUpdater() {
  const updateStatus = ref<UpdateStatus>('idle')
  const latestVersion = ref('')
  const checkingUpdate = ref(false)
  const isDownloading = ref(false)
  const downloadPercentage = ref(0)
  const isUpdateReady = ref(false)
  const updateError = ref('')
  const downloadAssetURL = ref('')

  const unlisten = EventsOn('update:download-progress', (data: { percentage?: number }) => {
    downloadPercentage.value = Math.round(data?.percentage ?? 0)
  })
  // Only register the lifecycle hook inside a component; this keeps the
  // composable callable from unit tests without an active instance.
  if (getCurrentInstance()) {
    onUnmounted(() => unlisten?.())
  }

  async function checkUpdates() {
    updateError.value = ''
    checkingUpdate.value = true
    try {
      const info = await CheckForUpdates()
      latestVersion.value = info?.latestVersion ?? ''
      if (info?.hasUpdate) {
        updateStatus.value = 'available'
        downloadAssetURL.value = info.assetURL
      } else {
        updateStatus.value = 'up-to-date'
      }
      return info
    } catch (e: any) {
      updateError.value = String(e?.message ?? e)
      throw e
    } finally {
      checkingUpdate.value = false
    }
  }

  async function downloadUpdate() {
    if (!downloadAssetURL.value) return
    updateError.value = ''
    isDownloading.value = true
    downloadPercentage.value = 0
    try {
      await DownloadUpdate(downloadAssetURL.value)
      isUpdateReady.value = true
    } catch (e: any) {
      updateError.value = String(e?.message ?? e)
      throw e
    } finally {
      isDownloading.value = false
    }
  }

  function installUpdate() {
    updateStatus.value = 'restarting'
    updateError.value = ''
    return InstallAndRestart().catch((e: any) => {
      updateError.value = String(e?.message ?? e)
      updateStatus.value = 'available'
      throw e
    })
  }

  return {
    updateStatus,
    latestVersion,
    checkingUpdate,
    isDownloading,
    downloadPercentage,
    isUpdateReady,
    updateError,
    downloadAssetURL,
    checkUpdates,
    downloadUpdate,
    installUpdate,
  }
}
