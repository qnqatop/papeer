<template>
  <n-space vertical :size="16">
    <n-card v-if="!profileStore.activeProfileId" :title="t('topics.title')">
      <n-empty :description="t('common.selectProfileFirst')" />
    </n-card>

    <template v-else>
      <n-card v-if="loading">
        <div style="display: flex; justify-content: center; padding: 48px 0">
          <n-spin size="large" />
        </div>
      </n-card>

      <n-card v-else-if="topics.length === 0" :title="t('topics.title')">
        <n-empty :description="t('topics.empty')" />
      </n-card>

      <div v-else style="display: flex; gap: 16px; height: calc(100vh - 260px); min-height: 420px">
        <!-- Scatter plot -->
        <n-card style="flex: 1.4; overflow: hidden" content-style="padding: 8px; height: 100%">
          <svg :viewBox="`0 0 ${scatter.width} ${scatter.height}`" class="topics-scatter" preserveAspectRatio="xMidYMid meet">
            <circle
              v-for="pt in scatter.points"
              :key="pt.id"
              :cx="pt.x"
              :cy="pt.y"
              :r="pt.r"
              :fill="pt.color"
              :fill-opacity="selectedId === pt.id || selectedId === null ? 0.75 : 0.25"
              :stroke="selectedId === pt.id ? '#f8fafc' : pt.color"
              stroke-width="1.5"
              class="topics-scatter__point"
              @click="selectedId = pt.id"
            />
            <text
              v-for="pt in scatter.points"
              :key="'lbl' + pt.id"
              :x="pt.x"
              :y="pt.y - pt.r - 4"
              text-anchor="middle"
              class="topics-scatter__label"
              :opacity="selectedId === pt.id || selectedId === null ? 1 : 0.35"
            >{{ pt.label }}</text>
          </svg>
        </n-card>

        <!-- Detail panel -->
        <n-card style="width: 380px; overflow-y: auto" :title="selectedTopic ? selectedTopic.label : t('topics.selectHint')">
          <template v-if="selectedTopic">
            <n-space :size="6" style="margin-bottom: 12px" :wrap="true">
              <n-tag v-for="term in selectedTopic.top_terms" :key="term" size="small" round>{{ term }}</n-tag>
            </n-space>
            <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">
              {{ t('topics.paperCount', { count: selectedTopic.paper_ids.length }) }}
            </n-text>
            <div
              v-for="pid in selectedTopic.paper_ids"
              :key="pid"
              class="topics-paper-row"
              @click="openPaper(pid)"
            >
              <div style="font-size: 13px; font-weight: 500; line-height: 1.4">
                {{ paperMap.get(pid)?.title ?? ('#' + pid) }}
              </div>
              <n-text v-if="paperMap.get(pid)?.year" depth="3" style="font-size: 12px">
                {{ paperMap.get(pid)?.year }}
              </n-text>
            </div>
          </template>
        </n-card>
      </div>
    </template>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NCard, NEmpty, NSpace, NSpin, NText, NTag, useMessage } from 'naive-ui'
import { GetTopics } from '../../wailsjs/go/app/App'
import { topics as topicsNs, db } from '../../wailsjs/go/models'
import { useProfileStore } from '../stores/profile'
import { localizeBackendError } from '../utils/errors'
import { fetchApprovedDownloadedMap } from '../utils/paperMap'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const profileStore = useProfileStore()

const topics = ref<topicsNs.Topic[]>([])
const loading = ref(false)
const selectedId = ref<number | null>(null)
const paperMap = ref<Map<number, db.Paper>>(new Map())

const PALETTE = ['#7c5cff', '#22c9a6', '#f59e0b', '#f87171', '#38bdf8', '#a3e635', '#e879f9', '#fb923c', '#4ade80', '#818cf8', '#facc15', '#fb7185']

const selectedTopic = computed(() => topics.value.find(t2 => t2.id === selectedId.value) ?? null)

const scatter = computed(() => {
  const width = 600
  const height = 440
  const pad = 40
  if (topics.value.length === 0) return { width, height, points: [] as any[] }

  const xs = topics.value.map(t2 => t2.x)
  const ys = topics.value.map(t2 => t2.y)
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)
  const spanX = maxX - minX || 1
  const spanY = maxY - minY || 1
  const maxSize = Math.max(1, ...topics.value.map(t2 => t2.size))

  const points = topics.value.map((t2, i) => ({
    id: t2.id,
    label: t2.label,
    x: pad + ((t2.x - minX) / spanX) * (width - 2 * pad),
    y: pad + ((maxY - t2.y) / spanY) * (height - 2 * pad),
    r: 10 + Math.sqrt(t2.size / maxSize) * 34,
    color: PALETTE[i % PALETTE.length],
  }))
  return { width, height, points }
})

function openPaper(id: number) {
  router.push({ path: '/papers', query: { paper_id: String(id) } })
}

async function loadTopics() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  loading.value = true
  try {
    topics.value = (await GetTopics(pid, 0)) || []
    selectedId.value = topics.value.length > 0 ? topics.value[0].id : null
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
    topics.value = []
  } finally {
    loading.value = false
  }
}

async function loadPaperMap() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    paperMap.value = await fetchApprovedDownloadedMap(pid)
  } catch {
    paperMap.value = new Map()
  }
}

onMounted(() => {
  loadTopics()
  loadPaperMap()
})

watch(() => profileStore.activeProfileId, () => {
  selectedId.value = null
  loadTopics()
  loadPaperMap()
})
</script>

<style scoped>
.topics-scatter {
  width: 100%;
  height: 100%;
  display: block;
}

.topics-scatter__point {
  cursor: pointer;
  transition: fill-opacity 150ms ease;
}

.topics-scatter__label {
  font-size: 11px;
  fill: var(--text-secondary, #cbd5e1);
  pointer-events: none;
}

.topics-paper-row {
  padding: 8px 0;
  border-bottom: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08));
  cursor: pointer;
}

.topics-paper-row:hover {
  background: rgba(124, 92, 255, 0.06);
}
</style>
