<template>
  <n-space vertical :size="16">
    <!-- No active profile -->
    <n-card v-if="!profileStore.activeProfileId" :title="t('stats.title')">
      <n-empty :description="t('common.selectProfileFirst')" />
    </n-card>

    <template v-else>
      <!-- Axis filter -->
      <n-space align="center" style="margin-bottom: 4px">
        <n-select
          v-model:value="axisFilter"
          :options="axisOptions"
          :placeholder="t('stats.allAxes')"
          clearable
          size="small"
          style="width: 220px"
        />
      </n-space>

      <!-- Loading -->
      <n-card v-if="loading" :title="t('stats.title')">
        <div style="display: flex; justify-content: center; padding: 48px 0">
          <n-spin size="large" />
        </div>
      </n-card>

      <!-- Error -->
      <n-card v-else-if="error" :title="t('stats.title')">
        <n-text type="error">{{ error }}</n-text>
      </n-card>

      <!-- Data loaded -->
      <template v-else-if="stats">
        <!-- Year Distribution + trend + cumulative -->
        <n-card :title="t('stats.yearDistribution')">
          <template #header-extra>
            <n-space :size="8">
              <n-button size="tiny" :disabled="yearEntries.length === 0" @click="exportYearChartPng">
                {{ t('v2.analysis.exportPng') }}
              </n-button>
              <n-button size="tiny" type="primary" :loading="generatingParagraph" :disabled="yearEntries.length === 0" @click="insertIntoReport">
                {{ t('v2.analysis.insertIntoReport') }}
              </n-button>
            </n-space>
          </template>

          <n-empty v-if="yearEntries.length === 0" :description="t('stats.noYearData')" />
          <template v-else>
            <svg
              ref="yearSvgRef"
              :viewBox="`0 0 ${chart.width} ${chart.height}`"
              class="year-chart"
              preserveAspectRatio="xMidYMid meet"
            >
              <!-- gridlines -->
              <line
                v-for="gl in chart.gridlines"
                :key="'gl' + gl.y"
                :x1="chart.marginLeft"
                :x2="chart.width - chart.marginRight"
                :y1="gl.y"
                :y2="gl.y"
                class="year-chart__grid"
              />
              <text
                v-for="gl in chart.gridlines"
                :key="'glt' + gl.y"
                :x="chart.marginLeft - 6"
                :y="gl.y + 3"
                class="year-chart__axis-label"
                text-anchor="end"
              >{{ gl.label }}</text>

              <!-- bars -->
              <rect
                v-for="b in chart.bars"
                :key="'bar' + b.year"
                :x="b.x"
                :y="b.y"
                :width="b.width"
                :height="b.height"
                class="year-chart__bar"
              />
              <text
                v-for="b in chart.bars"
                :key="'lbl' + b.year"
                :x="b.x + b.width / 2"
                :y="chart.height - chart.marginBottom + 14"
                class="year-chart__axis-label"
                text-anchor="middle"
              >{{ b.year }}</text>

              <!-- trend line -->
              <line
                :x1="chart.trend.x1" :y1="chart.trend.y1"
                :x2="chart.trend.x2" :y2="chart.trend.y2"
                class="year-chart__trend"
              />

              <!-- cumulative curve -->
              <path :d="chart.cumulativePath" class="year-chart__cumulative" fill="none" />

              <!-- median marker -->
              <line
                v-if="chart.medianX !== null"
                :x1="chart.medianX" :x2="chart.medianX"
                :y1="chart.marginTop" :y2="chart.height - chart.marginBottom"
                class="year-chart__median"
              />
              <text
                v-if="chart.medianX !== null"
                :x="chart.medianX + 4" :y="chart.marginTop + 10"
                class="year-chart__median-label"
              >{{ t('v2.analysis.median', { year: medianYear }) }}</text>
            </svg>
            <n-space :size="16" style="margin-top: 8px" align="center">
              <n-space :size="6" align="center">
                <span class="legend-swatch legend-swatch--bar" />
                <n-text depth="3" style="font-size: 12px">{{ t('stats.yearDistribution') }}</n-text>
              </n-space>
              <n-space :size="6" align="center">
                <span class="legend-swatch legend-swatch--trend" />
                <n-text depth="3" style="font-size: 12px">{{ t('v2.analysis.trendLine') }}</n-text>
              </n-space>
              <n-space :size="6" align="center">
                <span class="legend-swatch legend-swatch--cumulative" />
                <n-text depth="3" style="font-size: 12px">{{ t('v2.analysis.cumulative') }}</n-text>
              </n-space>
            </n-space>
          </template>
        </n-card>

        <!-- Top Authors -->
        <n-card v-if="stats.top_authors && stats.top_authors.length > 0" :title="t('v2.analysis.topAuthors')">
          <div style="display: flex; flex-direction: column; gap: 6px">
            <div
              v-for="entry in stats.top_authors"
              :key="entry.name"
              style="display: flex; align-items: center; gap: 8px"
            >
              <span style="width: 160px; text-align: right; font-size: 13px; flex-shrink: 0" class="ellipsis" :title="entry.name">
                {{ entry.name }}
              </span>
              <div style="flex: 1; background: var(--surface-border, rgba(148, 163, 184, 0.12)); border-radius: 4px; height: 20px">
                <div
                  :style="{
                    width: barWidth(entry.count, maxAuthorCount) + '%',
                    minWidth: entry.count > 0 ? '2px' : '0',
                    height: '100%',
                    background: 'var(--color-primary-500, #7c5cff)',
                    borderRadius: '4px',
                  }"
                />
              </div>
              <span style="width: 24px; text-align: left; font-size: 13px; color: var(--text-tertiary, #94a3b8); flex-shrink: 0">
                {{ entry.count }}
              </span>
            </div>
          </div>
        </n-card>

        <!-- Citation Count Distribution -->
        <n-card v-if="stats.citation_buckets && stats.citation_buckets.length > 0" :title="t('v2.analysis.citationBuckets')">
          <div style="display: flex; flex-direction: column; gap: 6px">
            <div
              v-for="entry in stats.citation_buckets"
              :key="entry.label"
              style="display: flex; align-items: center; gap: 8px"
            >
              <span style="width: 56px; text-align: right; font-weight: 600; flex-shrink: 0; font-size: 13px">
                {{ entry.label }}
              </span>
              <div style="flex: 1; background: var(--surface-border, rgba(148, 163, 184, 0.12)); border-radius: 4px; height: 20px">
                <div
                  :style="{
                    width: barWidth(entry.count, maxBucketCount) + '%',
                    minWidth: entry.count > 0 ? '2px' : '0',
                    height: '100%',
                    background: 'var(--color-accent-500, #22c9a6)',
                    borderRadius: '4px',
                  }"
                />
              </div>
              <span style="width: 36px; text-align: left; font-size: 13px; color: var(--text-tertiary, #94a3b8); flex-shrink: 0">
                {{ entry.count }}
              </span>
            </div>
          </div>
        </n-card>
      </template>

      <!-- No data at all -->
      <n-card v-else :title="t('stats.title')">
        <n-empty :description="t('stats.emptyState')" />
      </n-card>
    </template>

    <!-- Insert into report paragraph -->
    <n-modal v-model:show="showParagraphModal" preset="card" :title="t('v2.analysis.insertIntoReport')" style="width: 560px">
      <n-input
        type="textarea"
        readonly
        :value="reportParagraph"
        :autosize="{ minRows: 4, maxRows: 10 }"
      />
      <template #footer>
        <n-space justify="end">
          <n-button size="small" @click="copyParagraph">{{ t('common.copyToClipboard') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NCard, NEmpty, NSpace, NText, NSpin, NSelect, NButton, NModal, NInput,
  useMessage,
} from 'naive-ui'
import { GetStats, AxesWithPapers, GenerateYearDistributionParagraph } from '../../wailsjs/go/app/App'
import { app, db } from '../../wailsjs/go/models'
import { useProfileStore } from '../stores/profile'
import { useFirstVisitHint } from '../composables/useFirstVisitHint'
import { localizeBackendError } from '../utils/errors'
import { downloadSvgAsPng } from '../utils/svgExport'

const { t } = useI18n()
const message = useMessage()
const profileStore = useProfileStore()

useFirstVisitHint('stats', {
  steps: () => [{
    popover: { title: t('hints.stats.title'), description: t('hints.stats.description') },
  }],
})

const stats = ref<app.StatsResult | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const axisFilter = ref<number | null>(null)
const axes = ref<db.Axis[]>([])
const yearSvgRef = ref<SVGSVGElement | null>(null)
const showParagraphModal = ref(false)
const reportParagraph = ref('')
const generatingParagraph = ref(false)

const axisOptions = computed(() =>
  axes.value.map(a => ({ label: a.axis_key, value: a.id }))
)

// Year distribution: sorted ascending, gaps between min/max filled with 0 so
// the cumulative curve and trend line aren't visually misleading.
const yearEntries = computed(() => {
  const yd = stats.value?.year_distribution ?? {}
  const years = Object.keys(yd).map(Number)
  if (years.length === 0) return []
  const min = Math.min(...years)
  const max = Math.max(...years)
  const out: { year: number; count: number }[] = []
  for (let y = min; y <= max; y++) {
    out.push({ year: y, count: yd[y] ?? 0 })
  }
  return out
})

const maxAuthorCount = computed(() => Math.max(1, ...(stats.value?.top_authors ?? []).map(e => e.count)))
const maxBucketCount = computed(() => Math.max(1, ...(stats.value?.citation_buckets ?? []).map(e => e.count)))

function barWidth(count: number, max: number): number {
  return (count / max) * 100
}

const medianYear = computed<number | null>(() => {
  const entries = yearEntries.value
  const total = entries.reduce((s, e) => s + e.count, 0)
  if (total === 0) return null
  let running = 0
  const half = total / 2
  for (const e of entries) {
    running += e.count
    if (running >= half) return e.year
  }
  return entries[entries.length - 1]?.year ?? null
})

// Chart geometry — computed once so the template stays declarative.
const chart = computed(() => {
  const width = 680
  const height = 220
  const marginLeft = 34
  const marginRight = 12
  const marginTop = 12
  const marginBottom = 28
  const plotWidth = width - marginLeft - marginRight
  const plotHeight = height - marginTop - marginBottom
  const entries = yearEntries.value
  const n = entries.length

  if (n === 0) {
    return {
      width, height, marginLeft, marginRight, marginTop, marginBottom,
      bars: [] as { year: number; x: number; y: number; width: number; height: number }[],
      gridlines: [] as { y: number; label: string }[],
      cumulativePath: '',
      trend: { x1: 0, y1: 0, x2: 0, y2: 0 },
      medianX: null as number | null,
    }
  }

  const maxCount = Math.max(1, ...entries.map(e => e.count))
  const slot = plotWidth / n
  const barW = Math.max(2, slot * 0.65)
  const yFor = (count: number) => marginTop + plotHeight - (count / maxCount) * plotHeight
  const xCenterFor = (i: number) => marginLeft + slot * i + slot / 2

  const bars = entries.map((e, i) => ({
    year: e.year,
    x: xCenterFor(i) - barW / 2,
    y: yFor(e.count),
    width: barW,
    height: marginTop + plotHeight - yFor(e.count),
  }))

  // 4 evenly spaced gridlines on the count axis.
  const gridlines = [0, 0.25, 0.5, 0.75, 1].map(f => ({
    y: marginTop + plotHeight * (1 - f),
    label: String(Math.round(maxCount * f)),
  }))

  // Cumulative curve, normalized 0..1 of total (independent scale from bars).
  const total = entries.reduce((s, e) => s + e.count, 0) || 1
  let running = 0
  const cumPoints = entries.map((e, i) => {
    running += e.count
    const frac = running / total
    return [xCenterFor(i), marginTop + plotHeight * (1 - frac)] as [number, number]
  })
  const cumulativePath = cumPoints.map((p, i) => (i === 0 ? 'M' : 'L') + p[0].toFixed(1) + ',' + p[1].toFixed(1)).join(' ')

  // Trend line: least-squares linear regression of count vs. index.
  const xs = entries.map((_, i) => i)
  const ys = entries.map(e => e.count)
  const meanX = xs.reduce((s, v) => s + v, 0) / n
  const meanY = ys.reduce((s, v) => s + v, 0) / n
  let num = 0
  let den = 0
  for (let i = 0; i < n; i++) {
    num += (xs[i] - meanX) * (ys[i] - meanY)
    den += (xs[i] - meanX) ** 2
  }
  const slope = den === 0 ? 0 : num / den
  const intercept = meanY - slope * meanX
  const trendAt = (i: number) => Math.max(0, intercept + slope * i)
  const trend = {
    x1: xCenterFor(0), y1: yFor(trendAt(0)),
    x2: xCenterFor(n - 1), y2: yFor(trendAt(n - 1)),
  }

  // Median marker x position.
  let medianX: number | null = null
  const half = total / 2
  let acc = 0
  for (let i = 0; i < n; i++) {
    acc += entries[i].count
    if (acc >= half) { medianX = xCenterFor(i); break }
  }

  return { width, height, marginLeft, marginRight, marginTop, marginBottom, bars, gridlines, cumulativePath, trend, medianX }
})

async function fetchStats() {
  const pid = profileStore.activeProfileId
  if (!pid) {
    stats.value = null
    return
  }
  loading.value = true
  error.value = null
  try {
    stats.value = await GetStats(pid, axisFilter.value ?? null)
  } catch (e: any) {
    error.value = localizeBackendError(e, t)
    stats.value = null
  } finally {
    loading.value = false
  }
}

async function loadAxes() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    axes.value = await AxesWithPapers(pid) || []
  } catch {
    // ignore
  }
}

async function insertIntoReport() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  generatingParagraph.value = true
  try {
    reportParagraph.value = await GenerateYearDistributionParagraph(pid, axisFilter.value ?? null)
    showParagraphModal.value = true
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
  } finally {
    generatingParagraph.value = false
  }
}

async function copyParagraph() {
  try {
    await navigator.clipboard.writeText(reportParagraph.value)
    message.success(t('common.copied'))
  } catch {
    message.error(t('common.copyFailed'))
  }
}

async function exportYearChartPng() {
  if (!yearSvgRef.value) return
  try {
    await downloadSvgAsPng(yearSvgRef.value, 'year-distribution.png')
  } catch {
    message.error(t('v2.analysis.exportPngFailed'))
  }
}

onMounted(() => {
  loadAxes()
  fetchStats()
})

watch(() => profileStore.activeProfileId, () => {
  axisFilter.value = null
  loadAxes()
  fetchStats()
})

watch(axisFilter, fetchStats)
</script>

<style scoped>
.ellipsis {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.year-chart {
  width: 100%;
  height: 220px;
  display: block;
}

.year-chart__grid {
  stroke: var(--surface-divider, rgba(148, 163, 184, 0.08));
  stroke-width: 1;
}

.year-chart__axis-label {
  font-size: 9px;
  fill: var(--text-tertiary, #94a3b8);
}

.year-chart__bar {
  fill: var(--color-success-500, #22c55e);
  opacity: 0.85;
}

.year-chart__trend {
  stroke: var(--color-secondary-500, #f59e0b);
  stroke-width: 1.5;
  stroke-dasharray: 5 4;
}

.year-chart__cumulative {
  stroke: var(--color-primary-400, #9880ff);
  stroke-width: 2;
}

.year-chart__median {
  stroke: var(--color-error-400, #f87171);
  stroke-width: 1;
  stroke-dasharray: 3 3;
}

.year-chart__median-label {
  font-size: 9px;
  fill: var(--color-error-400, #f87171);
}

.legend-swatch {
  display: inline-block;
  width: 14px;
  height: 3px;
  border-radius: 2px;
}

.legend-swatch--bar { background: var(--color-success-500, #22c55e); }
.legend-swatch--trend { background: var(--color-secondary-500, #f59e0b); }
.legend-swatch--cumulative { background: var(--color-primary-400, #9880ff); }
</style>
