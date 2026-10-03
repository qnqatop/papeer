<template>
  <n-popover trigger="click" placement="bottom-end" :width="360">
    <template #trigger>
      <n-badge :value="activeCount" :show="activeCount > 0" :offset="[-4, 4]">
        <n-button quaternary circle size="small">
          <template #icon>
            <n-icon :component="SyncOutline" :class="{ spinning: activeCount > 0 }" />
          </template>
        </n-button>
      </n-badge>
    </template>

    <n-space vertical :size="8">
      <n-text strong style="font-size: 14px">{{ t('v2.header.backgroundTasks') }}</n-text>

      <template v-if="activeCount === 0 && !hasDownloadLog && !hasHistory">
        <n-text depth="3" style="font-size: 13px">{{ t('v2.header.noTasks') }}</n-text>
      </template>

      <template v-else>
        <!-- Search task -->
        <div v-if="progressStore.searching" style="padding: 8px 0">
          <n-space justify="space-between" align="center">
            <n-text style="font-size: 13px">{{ t('sidebar.searching') }}</n-text>
            <n-text depth="3" style="font-size: 12px">{{ searchPercent }}%</n-text>
          </n-space>
          <n-progress
            type="line"
            :percentage="searchPercent"
            :height="4"
            :show-indicator="false"
            :processing="true"
            style="margin-top: 4px"
          />
          <n-text depth="3" style="font-size: 11px; margin-top: 2px; display: block">
            {{ progressStore.lastEvent }}
          </n-text>
        </div>

        <!-- Download task — stays visible after completion so the user can
             review which papers succeeded/failed. Cleared when a new download
             starts (the store resets downloadEvents on the first progress event). -->
        <div v-if="progressStore.downloading || hasDownloadLog" style="padding: 8px 0">
          <n-space justify="space-between" align="center">
            <n-text style="font-size: 13px">
              {{ progressStore.downloading ? t('sidebar.downloading') : t('v2.header.downloadComplete') }}
            </n-text>
            <n-text depth="3" style="font-size: 12px">
              {{ progressStore.current }} / {{ progressStore.total }}
            </n-text>
          </n-space>
          <n-progress
            v-if="progressStore.downloading"
            type="line"
            :percentage="downloadPercent"
            :height="4"
            :show-indicator="false"
            :processing="true"
            style="margin-top: 4px"
          />
          <!-- Per-paper download log with per-source attempt chips. -->
          <download-log max-height="240px" style="margin-top: 6px" />
        </div>

        <!-- Review draft -->
        <div v-if="progressStore.reviewRunning" style="padding: 8px 0">
          <n-space justify="space-between" align="center">
            <n-text style="font-size: 13px">{{ t('v2.header.draftingReview') }}</n-text>
            <n-text depth="3" style="font-size: 12px">
              {{ progressStore.reviewCurrent }} / {{ progressStore.reviewTotal }}
            </n-text>
          </n-space>
          <n-progress
            type="line"
            :percentage="reviewPercent"
            :height="4"
            :show-indicator="false"
            :processing="true"
            style="margin-top: 4px"
          />
          <n-text depth="3" style="font-size: 11px; margin-top: 2px; display: block">
            {{ progressStore.reviewLabel }}
          </n-text>
        </div>

        <!-- Radar -->
        <div v-if="progressStore.radarRunning" style="padding: 8px 0">
          <n-space align="center" :size="8">
            <n-spin :size="14" />
            <n-text style="font-size: 13px">{{ t('monitoring.running') }}</n-text>
          </n-space>
        </div>

        <!-- Failed downloads: survive the per-run log reset until retried
             successfully or dismissed. -->
        <div v-if="progressStore.failedDownloads.length > 0" class="task-section">
          <n-text style="font-size: 13px">
            {{ t('v2.header.failedDownloads') }} ({{ progressStore.failedDownloads.length }})
          </n-text>
          <div class="failed-list">
            <div v-for="f in progressStore.failedDownloads" :key="f.paper_id" class="failed-row">
              <div class="failed-text">
                <n-ellipsis :line-clamp="1" style="font-size: 12px">{{ f.title || `#${f.paper_id}` }}</n-ellipsis>
                <n-text depth="3" style="font-size: 11px; display: block" :title="f.error">
                  {{ downloadFailReason(f.error, t) }}
                </n-text>
              </div>
              <n-button size="tiny" secondary :loading="retrying.has(f.paper_id)" @click="retry(f.paper_id)">
                {{ t('v2.header.retry') }}
              </n-button>
              <n-button size="tiny" quaternary :title="t('v2.header.dismiss')" @click="progressStore.dismissFailedDownload(f.paper_id)">
                ×
              </n-button>
            </div>
          </div>
        </div>

        <!-- Recent radar runs (this session) -->
        <div v-if="progressStore.radarRuns.length > 0" class="task-section">
          <n-text style="font-size: 13px">{{ t('v2.header.radarRuns') }}</n-text>
          <n-text
            v-for="r in progressStore.radarRuns"
            :key="r.at"
            depth="3"
            style="font-size: 12px; display: block"
          >
            {{ t('v2.header.radarRunLine', { time: formatTime(r.at), count: r.new_papers }) }}
          </n-text>
        </div>
      </template>
    </n-space>
  </n-popover>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NBadge, NButton, NEllipsis, NIcon, NPopover, NProgress,
  NSpace, NSpin, NText, useMessage,
} from 'naive-ui'
import { SyncOutline } from '@vicons/ionicons5'
import { useProgressStore } from '../../stores/progress'
import DownloadLog from '../DownloadLog.vue'
import { DownloadPaper } from '../../../wailsjs/go/app/App'
import { downloadFailReason, localizeBackendError } from '../../utils/errors'

const { t, locale } = useI18n()
const progressStore = useProgressStore()
const message = useMessage()

const hasHistory = computed(() =>
  progressStore.failedDownloads.length > 0 || progressStore.radarRuns.length > 0,
)

// Paper ids whose retry RPC is in flight.
const retrying = ref(new Set<number>())

async function retry(paperId: number) {
  retrying.value = new Set(retrying.value).add(paperId)
  try {
    await DownloadPaper(paperId)
    // The download is running now; its progress shows above, and a new
    // failure brings the row back.
    progressStore.dismissFailedDownload(paperId)
    message.info(t('download.retryStarted'))
  } catch (e) {
    message.error(localizeBackendError(e, t))
  } finally {
    const next = new Set(retrying.value)
    next.delete(paperId)
    retrying.value = next
  }
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit' })
}

const activeCount = computed(() => {
  let count = 0
  if (progressStore.searching) count++
  if (progressStore.downloading) count++
  if (progressStore.reviewRunning) count++
  if (progressStore.radarRunning) count++
  return count
})

// Keep the finished download log reachable in the popover after the run ends.
// downloadEvents are cleared by the store when the next download starts.
const hasDownloadLog = computed(() => progressStore.downloadEvents.length > 0)

const searchPercent = computed(() => {
  if (progressStore.total === 0) return 0
  return Math.round((progressStore.current / progressStore.total) * 100)
})

const downloadPercent = computed(() => {
  if (progressStore.total === 0) return 0
  return Math.round((progressStore.current / progressStore.total) * 100)
})

const reviewPercent = computed(() => {
  if (progressStore.reviewTotal === 0) return 0
  return Math.round((progressStore.reviewCurrent / progressStore.reviewTotal) * 100)
})
</script>

<style scoped>
.task-section {
  padding: 8px 0;
  border-top: 1px solid var(--surface-divider);
}
.failed-list {
  max-height: 180px;
  overflow-y: auto;
  margin-top: 4px;
}
.failed-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 0;
}
.failed-text {
  flex: 1;
  min-width: 0;
}
.spinning {
  animation: spin 1.2s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
