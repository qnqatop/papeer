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

      <template v-if="activeCount === 0 && !hasDownloadLog">
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
      </template>
    </n-space>
  </n-popover>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NBadge, NButton, NIcon, NPopover, NProgress,
  NSpace, NSpin, NText,
} from 'naive-ui'
import { SyncOutline } from '@vicons/ionicons5'
import { useProgressStore } from '../../stores/progress'
import DownloadLog from '../DownloadLog.vue'

const { t } = useI18n()
const progressStore = useProgressStore()

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
.spinning {
  animation: spin 1.2s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
