<template>
  <n-drawer v-model:show="visible" :width="600" placement="right">
    <n-drawer-content :title="t('v2.searchLog.drawerTitle')" closable>
      <!-- Status bar -->
      <n-space align="center" style="margin-bottom: 12px">
        <n-tag :type="progressStore.searching ? 'warning' : events.length > 0 ? 'success' : 'default'" size="small">
          {{ progressStore.searching ? t('searchLog.searching') : events.length > 0 ? t('searchLog.complete') : t('searchLog.idle') }}
        </n-tag>
        <n-text depth="3" style="font-size: 13px" v-if="events.length > 0">
          {{ t('searchLog.events', { count: events.length }) }}
        </n-text>
      </n-space>

      <!-- Filters -->
      <n-space :size="8" style="margin-bottom: 12px">
        <n-input
          v-model:value="filterText"
          :placeholder="t('searchLog.filterPlaceholder')"
          clearable
          size="small"
          style="width: 180px"
        />
        <n-select
          v-model:value="filterType"
          :options="typeOptions"
          :placeholder="t('searchLog.allTypes')"
          clearable
          size="small"
          style="width: 140px"
        />
        <n-button size="small" quaternary @click="progressStore.resetSearch()" :disabled="progressStore.searching">
          {{ t('common.clear') }}
        </n-button>
      </n-space>

      <!-- Summary cards -->
      <n-grid :cols="4" :x-gap="8" style="margin-bottom: 12px" v-if="summary.providers > 0">
        <n-gi>
          <n-card size="small" :bordered="true">
            <n-statistic :label="t('searchLog.providersCalled')" :value="summary.providers" />
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="true">
            <n-statistic :label="t('searchLog.rawPapers')" :value="summary.rawTotal" />
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="true">
            <n-statistic :label="t('searchLog.afterDedup')" :value="summary.dedupedTotal" />
          </n-card>
        </n-gi>
        <n-gi>
          <n-card size="small" :bordered="true">
            <n-statistic :label="t('searchLog.errors')" :value="summary.errors" />
          </n-card>
        </n-gi>
      </n-grid>

      <!-- Provider breakdown -->
      <n-card size="small" :title="t('searchLog.providerBreakdown')" style="margin-bottom: 12px" v-if="providerStats.length > 0">
        <n-space :size="6" :wrap="true">
          <n-tag
            v-for="ps in providerStats"
            :key="ps.name"
            :type="ps.errors > 0 ? 'error' : 'success'"
            size="small"
          >
            {{ ps.name }}: {{ ps.total }}
            <template v-if="ps.errors > 0"> / {{ ps.errors }} err</template>
          </n-tag>
        </n-space>
      </n-card>

      <!-- Event stream -->
      <n-scrollbar style="max-height: calc(100vh - 320px)" ref="scrollRef" v-if="events.length > 0">
        <div
          v-for="(ev, idx) in filteredEvents"
          :key="idx"
          style="padding: 2px 0; font-size: 12px; line-height: 1.6; font-family: monospace"
        >
          <template v-if="ev.type === 'query_start'">
            <n-text strong>[{{ ev.axis }}] {{ t('searchLog.query') }} "{{ ev.query }}"</n-text>
          </template>
          <template v-else-if="ev.type === 'provider_done'">
            <n-text style="color: var(--color-success-500, #22c55e)">
              &nbsp;&nbsp;{{ ev.provider }}: {{ ev.count }} papers
              <n-text depth="3" v-if="ev.duration_ms"> ({{ ev.duration_ms }}ms)</n-text>
            </n-text>
          </template>
          <template v-else-if="ev.type === 'provider_error'">
            <n-text style="color: var(--color-error-500, #ef4444)">
              &nbsp;&nbsp;{{ ev.provider }}: {{ t('searchLog.error') }} {{ ev.error }}
            </n-text>
          </template>
          <template v-else-if="ev.type === 'query_done'">
            <n-text depth="2">
              &nbsp;&nbsp;{{ t('searchLog.queryTotal') }} {{ ev.count }} papers
            </n-text>
          </template>
          <template v-else-if="ev.type === 'axis_done'">
            <n-text strong type="info">
              [{{ ev.axis }}] {{ t('searchLog.done') }} {{ ev.total }} {{ t('searchLog.saved') }}
              <template v-if="ev.raw_count"> ({{ ev.raw_count }} {{ t('searchLog.raw') }} {{ ev.total }})</template>
            </n-text>
          </template>
          <template v-else-if="ev.type === 'ai_log'">
            <n-text type="info" strong>&nbsp;&nbsp;[AI]: {{ ev.query }}</n-text>
          </template>
        </div>
      </n-scrollbar>

      <n-empty v-else :description="t('searchLog.emptyState')" />
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NDrawer, NDrawerContent, NCard, NEmpty, NSpace, NText, NTag,
  NInput, NSelect, NButton, NScrollbar, NGrid, NGi, NStatistic,
} from 'naive-ui'
import { useProgressStore } from '../stores/progress'

const { t } = useI18n()
const progressStore = useProgressStore()

const visible = defineModel<boolean>('show', { default: false })

const scrollRef = ref<InstanceType<typeof NScrollbar> | null>(null)
const filterText = ref('')
const filterType = ref<string | null>(null)

const typeOptions = computed(() => [
  { label: 'AI', value: 'ai_log' },
  { label: t('searchLog.typeQueryStart'), value: 'query_start' },
  { label: t('searchLog.typeProviderDone'), value: 'provider_done' },
  { label: t('searchLog.typeProviderError'), value: 'provider_error' },
  { label: t('searchLog.typeQueryDone'), value: 'query_done' },
  { label: t('searchLog.typeAxisDone'), value: 'axis_done' },
])

const events = computed(() => progressStore.searchEvents)

const filteredEvents = computed(() => {
  let result = events.value
  if (filterType.value) {
    result = result.filter(e => e.type === filterType.value)
  }
  if (filterText.value) {
    const q = filterText.value.toLowerCase()
    result = result.filter(e =>
      e.axis?.toLowerCase().includes(q) ||
      e.query?.toLowerCase().includes(q) ||
      e.provider?.toLowerCase().includes(q) ||
      e.error?.toLowerCase().includes(q)
    )
  }
  return result
})

const summary = computed(() => {
  const evts = events.value
  const providerDone = evts.filter(e => e.type === 'provider_done')
  const providerErrors = evts.filter(e => e.type === 'provider_error')
  const axisDone = evts.filter(e => e.type === 'axis_done')
  return {
    providers: providerDone.length + providerErrors.length,
    rawTotal: axisDone.reduce((sum, e) => sum + (e.raw_count || 0), 0),
    dedupedTotal: axisDone.reduce((sum, e) => sum + e.total, 0),
    errors: providerErrors.length,
  }
})

const providerStats = computed(() => {
  const map: Record<string, { total: number; errors: number; totalMs: number; calls: number }> = {}
  for (const ev of events.value) {
    if (ev.type !== 'provider_done' && ev.type !== 'provider_error') continue
    if (!ev.provider) continue
    if (!map[ev.provider]) map[ev.provider] = { total: 0, errors: 0, totalMs: 0, calls: 0 }
    const s = map[ev.provider]
    if (ev.type === 'provider_done') {
      s.total += ev.count; s.calls++; s.totalMs += ev.duration_ms || 0
    } else {
      s.errors++; s.calls++; s.totalMs += ev.duration_ms || 0
    }
  }
  return Object.entries(map).map(([name, s]) => ({
    name, total: s.total, errors: s.errors,
    avgMs: s.calls > 0 ? Math.round(s.totalMs / s.calls) : 0,
  }))
})

watch(() => events.value.length, async () => {
  await nextTick()
  scrollRef.value?.scrollTo({ top: 999999 })
})
</script>
