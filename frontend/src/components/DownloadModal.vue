<template>
  <n-modal v-model:show="visible" preset="card" :title="t('download.title')" style="width: 640px; max-height: 80vh">
    <!-- No PDF directory warning -->
    <n-alert
      v-if="profileStore.activeProfile && !profileStore.activeProfile.pdf_dir"
      type="warning"
      :title="t('download.noPdfDirTitle')"
      style="margin-bottom: 16px"
    >
      {{ t('download.noPdfDirDesc') }}
    </n-alert>

    <!-- Controls -->
    <n-space align="center" style="margin-bottom: 16px">
      <n-button
        type="primary"
        :disabled="progressStore.downloading || !profileStore.activeProfile?.pdf_dir"
        :loading="checkingCount"
        @click="checkAndConfirm"
      >
        {{ t('download.download') }}
      </n-button>
      <n-button
        :disabled="!progressStore.downloading"
        @click="cancelDownload"
      >
        {{ t('common.cancel') }}
      </n-button>
      <n-text v-if="progressStore.lastEvent" depth="3" style="font-size: 12px">
        {{ progressStore.lastEvent }}
      </n-text>
    </n-space>

    <!-- Progress bar -->
    <n-progress
      v-if="progressStore.total > 0"
      type="line"
      :percentage="percentage"
      :indicator-placement="'inside'"
      :processing="progressStore.downloading"
      style="margin-bottom: 8px"
    />

    <n-text v-if="progressStore.total > 0" style="display: block; margin-bottom: 16px; font-size: 13px">
      {{ t('download.papersProcessed', { current: progressStore.current, total: progressStore.total }) }}
    </n-text>

    <!-- Completion summary -->
    <n-alert
      v-if="completed && !progressStore.downloading"
      :type="failCount > 0 ? 'warning' : 'success'"
      style="margin-bottom: 16px"
    >
      <n-space align="center" justify="space-between" style="width: 100%">
        <span>{{ t('download.completeMessage', { done: doneCount, fail: failCount, total: progressStore.total }) }}</span>
        <n-button
          v-if="failCount > 0"
          size="tiny"
          @click="exportFailed"
          :loading="exportingFailed"
        >
          {{ t('download.exportFailed') }}
        </n-button>
      </n-space>
    </n-alert>

    <!-- Per-paper log (shared with the header BackgroundTasksIndicator popover). -->
    <download-log />

    <!-- Confirm sub-dialog -->
    <n-modal v-model:show="showConfirm" preset="dialog" :title="t('download.confirmTitle')" :positive-text="t('download.startDownload')" :negative-text="t('common.cancel')" @positive-click="doDownload">
      <n-text>
        {{ t('download.confirmMessage', { count: approvedCount }) }}
        <n-text code>{{ profileStore.activeProfile?.pdf_dir }}</n-text>?
      </n-text>
    </n-modal>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NModal, NButton, NSpace, NProgress, NText, NAlert,
  useMessage,
} from 'naive-ui'
import { useProfileStore } from '../stores/profile'
import { useProgressStore } from '../stores/progress'
import { DownloadApproved, CancelOperation, ListPapers, ExportFailedDownloads, SaveExportFile } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'
import { isInvalidEmailError } from '../utils/errors'
import DownloadLog from './DownloadLog.vue'

const { t } = useI18n()
const message = useMessage()
const profileStore = useProfileStore()
const progressStore = useProgressStore()

const visible = defineModel<boolean>('show', { default: false })

const showConfirm = ref(false)
const approvedCount = ref(0)
const checkingCount = ref(false)
const exportingFailed = ref(false)

async function exportFailed() {
  if (!profileStore.activeProfileId) return
  exportingFailed.value = true
  try {
    const content = await ExportFailedDownloads(profileStore.activeProfileId)
    if (!content) {
      message.info(t('download.exportFailedEmpty'))
      return
    }
    await SaveExportFile('failed_downloads.txt', content)
  } catch (e: any) {
    message.error(e?.message || String(e))
  } finally {
    exportingFailed.value = false
  }
}

const percentage = computed(() => {
  if (progressStore.total === 0) return 0
  return Math.round((progressStore.current / progressStore.total) * 100)
})

const doneCount = computed(() => progressStore.downloadDone)
const failCount = computed(() => progressStore.downloadFailed)
const completed = computed(() =>
  progressStore.downloadEvents.some(e => e.type === 'complete')
)

async function checkAndConfirm() {
  if (!profileStore.activeProfileId) return
  checkingCount.value = true
  try {
    const result = await ListPapers(new db.PaperFilter({
      profile_id: profileStore.activeProfileId,
      status: 'approved',
      limit: 1,
      offset: 0,
    }))
    approvedCount.value = result.total
    if (approvedCount.value === 0) return
    showConfirm.value = true
  } finally {
    checkingCount.value = false
  }
}

async function doDownload() {
  if (!profileStore.activeProfileId) return
  progressStore.resetDownload()
  progressStore.downloading = true
  try {
    await DownloadApproved(profileStore.activeProfileId)
  } catch (e: any) {
    progressStore.downloading = false
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(e?.message || String(e))
  }
}

async function cancelDownload() {
  try { await CancelOperation() } catch { /* ignore */ }
}
</script>
