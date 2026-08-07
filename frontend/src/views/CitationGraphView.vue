<template>
  <n-space vertical :size="16">
    <!-- No active profile -->
    <n-card v-if="!profileStore.activeProfileId" :title="t('citationGraph.title')">
      <n-empty :description="t('citationGraph.noProfile')" />
    </n-card>

    <template v-else>
      <!-- Toolbar -->
      <n-space align="center" justify="space-between" :wrap="true">
        <n-space align="center" :size="12">
          <n-button
            type="primary"
            :loading="fetching"
            :disabled="fetching"
            @click="fetchCitations"
          >
            {{ hasEverFetched ? t('citationGraph.refetch') : t('citationGraph.fetchCitations') }}
          </n-button>

          <n-button v-if="fetching" size="small" quaternary type="warning" @click="cancelFetch">
            {{ t('common.cancel') }}
          </n-button>

          <n-button
            v-if="hasEverFetched"
            quaternary
            type="error"
            size="small"
            :disabled="fetching"
            @click="handleClear"
          >
            {{ t('citationGraph.clearData') }}
          </n-button>
        </n-space>

        <n-space v-if="state === 'hasData'" align="center" :size="12" :wrap="true">
          <n-text depth="3" style="font-size: 13px" :title="t('citationGraph.minMentionsHint')">
            {{ t('citationGraph.minMentions') }}:
          </n-text>
          <n-select v-model:value="pendingMinMentions" :options="minMentionsOptions" size="small" style="width: 84px" />
          <n-button size="small" @click="applyMinMentions">{{ t('common.apply') }}</n-button>

          <n-button-group v-if="nodeCountOk" size="small">
            <n-button :type="layoutMode === 'force' ? 'primary' : 'default'" @click="layoutMode = 'force'">
              {{ t('citationGraph.layoutForce') }}
            </n-button>
            <n-button :type="layoutMode === 'year' ? 'primary' : 'default'" @click="layoutMode = 'year'">
              {{ t('citationGraph.layoutYear') }}
            </n-button>
          </n-button-group>
        </n-space>
      </n-space>

      <!-- Progress -->
      <n-card v-if="fetching" size="small">
        <n-space vertical :size="8">
          <n-text>{{ t('citationGraph.fetching') }}</n-text>
          <n-progress
            type="line"
            :percentage="progressPercent"
            :height="6"
            :processing="true"
          />
          <n-text depth="3" style="font-size: 12px">
            {{ t('citationGraph.progress', { current: progress.current, total: progress.total, title: progress.title }) }}
          </n-text>
        </n-space>
      </n-card>

      <!-- Provider-block banner -->
      <n-alert v-if="blockReason" type="error" closable @close="blockReason = null">
        {{ t('citationGraph.fetchBlocked', { reason: blockReason }) }}
      </n-alert>

      <!-- Empty states (distinct per cause) -->
      <n-card v-if="!fetching && state === 'noPapers'" :title="t('citationGraph.title')">
        <n-empty :description="t('citationGraph.noPapers')" />
      </n-card>
      <n-card v-else-if="!fetching && state === 'neverFetched'" :title="t('citationGraph.title')">
        <n-empty :description="t('citationGraph.noData')" />
      </n-card>
      <n-card v-else-if="!fetching && state === 'emptyAfterFetch'" :title="t('citationGraph.title')">
        <n-empty :description="t('citationGraph.emptyAfterFetch')" />
      </n-card>
      <n-card v-else-if="!fetching && state === 'loading'">
        <div style="display: flex; justify-content: center; padding: 48px 0">
          <n-spin size="large" />
        </div>
      </n-card>

      <!-- Graph -->
      <template v-else-if="state === 'hasData' && !fetching">
        <!-- Stats bar -->
        <n-space :size="16" :wrap="true">
          <n-tag type="info">{{ t('citationGraph.nodes', { count: graphData!.nodes.length }) }}</n-tag>
          <n-tag type="info">{{ t('citationGraph.edges', { count: graphData!.edges.length }) }}</n-tag>
          <n-tag type="success">{{ t('citationGraph.internalPapers') }}: {{ internalCount }}</n-tag>
          <n-tag type="warning">{{ t('citationGraph.externalPapers') }}: {{ externalCount }}</n-tag>
          <n-tag v-if="graphData!.stats" type="default">
            {{ t('citationGraph.processed', { processed: graphData!.stats.papers_processed, eligible: graphData!.stats.papers_eligible }) }}
          </n-tag>
        </n-space>

        <n-alert v-if="!nodeCountOk" type="info">
          {{ t('citationGraph.simplified', { count: graphData!.nodes.length }) }}
          <n-button text type="primary" @click="emit('switch-tab', 'topics')" style="margin-left: 8px">
            {{ t('citationGraph.goToTopics') }}
          </n-button>
        </n-alert>

        <div v-else style="display: flex; gap: 16px; height: calc(100vh - 320px); min-height: 400px">
          <n-card style="flex: 1; overflow: hidden" content-style="padding: 0; height: 100%">
            <div style="position: relative; width: 100%; height: 100%">
              <v-network-graph
                :key="layoutMode + ':' + graphData!.nodes.length"
                ref="graphRef"
                :nodes="vngNodes"
                :edges="vngEdges"
                :configs="vngConfigs"
                :layouts="vngLayouts"
                :event-handlers="eventHandlers"
                style="width: 100%; height: 100%"
              />
              <!-- Legend overlay -->
              <div class="graph-legend">
                <n-text depth="3" style="font-size: 11px; font-weight: 600; display: block; margin-bottom: 4px">
                  {{ t('citationGraph.graphLegend') }}
                </n-text>
                <div class="graph-legend__item">
                  <span class="graph-legend__dot graph-legend__dot--approved"></span>
                  <n-text depth="3">{{ t('citationGraph.approved') }}</n-text>
                </div>
                <div class="graph-legend__item">
                  <span class="graph-legend__dot graph-legend__dot--downloaded"></span>
                  <n-text depth="3">{{ t('citationGraph.downloaded') }}</n-text>
                </div>
                <div class="graph-legend__item">
                  <span class="graph-legend__dot graph-legend__dot--external"></span>
                  <n-text depth="3">{{ t('citationGraph.external') }}</n-text>
                </div>
              </div>
              <div v-if="layoutMode === 'year'" class="graph-year-hint">
                {{ t('citationGraph.layoutYearLimitation') }}
              </div>
            </div>
          </n-card>
        </div>
      </template>
    </template>

    <!-- External node info modal (internal nodes navigate directly instead) -->
    <n-modal v-model:show="showExternalModal" preset="card" :title="externalNode?.title || ''" style="width: 420px">
      <template v-if="externalNode">
        <n-space vertical :size="8">
          <div v-if="externalNode.authors?.length">
            <n-text strong>{{ t('papers.authors') }}</n-text>
            <n-text depth="2"> {{ externalNode.authors.join(', ') }}</n-text>
          </div>
          <div v-if="externalNode.year">
            <n-text strong>{{ t('papers.year') }}</n-text>
            <n-text depth="2"> {{ externalNode.year }}</n-text>
          </div>
          <div>
            <n-text strong>{{ t('citationGraph.mentionedBy', { count: externalNode.mention_count }) }}</n-text>
          </div>
        </n-space>
      </template>
      <template #footer>
        <n-space justify="end">
          <n-button size="small" @click="emit('switch-tab', 'coverage-gaps'); showExternalModal = false">
            {{ t('citationGraph.goToCoverageGaps') }}
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NCard, NEmpty, NSpace, NButton, NButtonGroup, NTag, NText, NProgress,
  NSelect, NModal, NAlert, NSpin,
  useMessage, useDialog,
} from 'naive-ui'
import { VNetworkGraph, defineConfigs } from 'v-network-graph'
import { ForceLayout } from 'v-network-graph/lib/force-layout'
import 'v-network-graph/lib/style.css'
import {
  FetchCitations,
  GetCitationGraph,
  ClearCitationData,
  CancelOperation,
} from '../../wailsjs/go/app/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useProfileStore } from '../stores/profile'
import type { app } from '../../wailsjs/go/models'
import { isInvalidEmailError, localizeBackendError } from '../utils/errors'
import { svgToPngBase64 } from '../utils/svgExport'

const { t } = useI18n()
const profileStore = useProfileStore()
const message = useMessage()
const dialog = useDialog()
const router = useRouter()

const emit = defineEmits<{ (e: 'switch-tab', tab: string): void }>()

const MAX_RENDERABLE_NODES = 200

const graphRef = ref<InstanceType<typeof VNetworkGraph> | null>(null)
const graphData = ref<app.CitationGraphData | null>(null)
const fetching = ref(false)
const pendingMinMentions = ref(2)
const appliedMinMentions = ref(2)
const layoutMode = ref<'force' | 'year'>('force')
const blockReason = ref<string | null>(null)
const showExternalModal = ref(false)
const externalNode = ref<app.GraphNode | null>(null)
const loadedOnce = ref(false)

const minMentionsOptions = [2, 3, 5, 10].map(v => ({ label: String(v), value: v }))

const progress = reactive({ current: 0, total: 0, title: '' })

const progressPercent = computed(() => {
  if (progress.total === 0) return 0
  return Math.round((progress.current / progress.total) * 100)
})

const internalCount = computed(() =>
  graphData.value?.nodes.filter(n => n.node_type === 'internal').length ?? 0
)
const externalCount = computed(() =>
  graphData.value?.nodes.filter(n => n.node_type === 'external').length ?? 0
)
const nodeCountOk = computed(() => (graphData.value?.nodes.length ?? 0) <= MAX_RENDERABLE_NODES)

const hasEverFetched = computed(() => (graphData.value?.stats.papers_processed ?? 0) > 0)

// Three distinct empty states instead of one generic "no data": never ran a
// fetch vs. no eligible papers at all vs. fetch ran but found no relations.
const state = computed<'loading' | 'noPapers' | 'neverFetched' | 'emptyAfterFetch' | 'hasData'>(() => {
  if (!loadedOnce.value) return 'loading'
  const s = graphData.value?.stats
  if (!s || s.papers_eligible === 0) return 'noPapers'
  if (s.papers_processed === 0) return 'neverFetched'
  if (s.papers_processed > 0 && graphData.value!.nodes.length === 0) return 'emptyAfterFetch'
  return 'hasData'
})

// Convert graph data to v-network-graph format.
const vngNodes = computed(() => {
  if (!graphData.value) return {}
  const nodes: Record<string, { name: string }> = {}
  for (const n of graphData.value.nodes) {
    nodes[n.id] = { name: n.label }
  }
  return nodes
})

const vngEdges = computed(() => {
  if (!graphData.value) return {}
  const edges: Record<string, { source: string; target: string }> = {}
  for (let i = 0; i < graphData.value.edges.length; i++) {
    const e = graphData.value.edges[i]
    edges[`e${i}`] = { source: e.source, target: e.target }
  }
  return edges
})

const nodeYearById = computed(() => {
  const map: Record<string, number | null> = {}
  for (const n of graphData.value?.nodes ?? []) {
    map[n.id] = n.year ?? null
  }
  return map
})

function getNodeColor(nodeId: string): string {
  const node = graphData.value?.nodes.find(n => n.id === nodeId)
  if (!node) return '#64748b'
  if (node.node_type === 'external') return '#f59e0b'
  if (node.status === 'downloaded') return '#8b5cf6'
  return '#22c55e'
}

function getNodeSize(nodeId: string): number {
  const node = graphData.value?.nodes.find(n => n.id === nodeId)
  if (!node) return 16
  if (node.node_type === 'external') {
    return Math.min(12 + node.mention_count * 4, 28)
  }
  const cit = node.citation_count || 0
  return Math.min(14 + Math.log2(cit + 1) * 3, 32)
}

function getNodeStrokeDasharray(nodeId: string): string | undefined {
  const node = graphData.value?.nodes.find(n => n.id === nodeId)
  if (node?.node_type === 'external') return '4 2'
  return undefined
}

// Layered-by-year: v-network-graph's ForceLayout only exposes {id,x,y} to
// createSimulation (not the original node data), so the year lookup has to
// happen via a closed-over id -> year map built from graphData. Toggling
// layoutMode remounts <v-network-graph> (via :key) so a fresh simulation is
// created with the new force set — the layout handler itself isn't reactive
// mid-simulation. This is a pragmatic approximation, not a true multi-row
// timeline layout: nodes without a year fall back to a middle band.
const vngConfigs = defineConfigs({
  view: {
    scalingObjects: true,
    minZoomLevel: 0.2,
    maxZoomLevel: 3,
    layoutHandler: new ForceLayout({
      positionFixedByDrag: false,
      positionFixedByClickWithAltKey: true,
      createSimulation: (d3: any, nodes: any, edges: any) => {
        const forceLink = d3.forceLink(edges).id((d: any) => d.id)
        const sim = d3
          .forceSimulation(nodes)
          .force('edge', forceLink.distance(90).strength(0.4))
          .force('charge', d3.forceManyBody().strength(-260))
          .force('collide', d3.forceCollide(36))
          .alphaMin(0.001)

        if (layoutMode.value === 'year') {
          const years = Object.values(nodeYearById.value).filter((y): y is number => y != null)
          const minY = years.length ? Math.min(...years) : 0
          const maxY = years.length ? Math.max(...years) : 1
          const span = Math.max(1, maxY - minY)
          const bandHeight = 480
          const yFor = (year: number | null) => {
            if (year == null) return bandHeight / 2
            return ((year - minY) / span) * bandHeight - bandHeight / 2
          }
          sim
            .force('y', d3.forceY((d: any) => yFor(nodeYearById.value[d.id] ?? null)).strength(0.4))
            .force('x', d3.forceX(0).strength(0.03))
        } else {
          sim.force('center', d3.forceCenter().strength(0.05))
        }
        return sim
      },
    }),
  },
  node: {
    normal: {
      color: (node: any) => getNodeColor(node),
      radius: (node: any) => getNodeSize(node),
      strokeWidth: 2,
      strokeColor: (node: any) => getNodeColor(node),
      strokeDasharray: (node: any) => getNodeStrokeDasharray(node),
    },
    hover: {
      color: '#9880ff',
    },
    label: {
      visible: true,
      fontSize: 11,
      color: '#94a3b8',
      directionAutoAdjustment: true,
      margin: 4,
    },
  },
  edge: {
    normal: {
      color: 'rgba(148, 163, 184, 0.15)',
      width: 1,
    },
    marker: {
      target: {
        type: 'arrow',
        width: 4,
        height: 4,
      },
    },
  },
})

const vngLayouts = ref({})

const eventHandlers = {
  'node:click': ({ node }: { node: string }) => {
    const found = graphData.value?.nodes.find(n => n.id === node)
    if (!found) return
    if (found.node_type === 'internal' && found.paper_id) {
      router.push({ path: '/papers', query: { paper_id: String(found.paper_id) } })
    } else {
      externalNode.value = found
      showExternalModal.value = true
    }
  },
}

function applyMinMentions() {
  appliedMinMentions.value = pendingMinMentions.value
  loadGraph()
}

async function fetchCitations() {
  const pid = profileStore.activeProfileId
  if (!pid) return

  fetching.value = true
  blockReason.value = null
  progress.current = 0
  progress.total = 0
  progress.title = ''

  try {
    await FetchCitations(pid)
  } catch (e: any) {
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(localizeBackendError(e, t))
    fetching.value = false
  }
}

async function cancelFetch() {
  try { await CancelOperation() } catch { /* ignore */ }
  // The backend doesn't emit a terminal citation:done on cancellation (the
  // worker just returns from its goroutine) — progress made so far is
  // already persisted (last_citation_fetch_at per paper), so the next
  // "Fetch Citations" click resumes automatically. Flip the local flag
  // ourselves and reload to show whatever was captured before cancelling.
  fetching.value = false
  message.info(t('citationGraph.cancelled'))
  loadGraph()
}

async function loadGraph() {
  const pid = profileStore.activeProfileId
  if (!pid) return

  try {
    graphData.value = await GetCitationGraph(pid, appliedMinMentions.value)
  } catch (e: any) {
    graphData.value = null
    message.error(localizeBackendError(e, t))
  } finally {
    loadedOnce.value = true
  }
}

function handleClear() {
  const pid = profileStore.activeProfileId
  if (!pid) return

  dialog.warning({
    title: t('citationGraph.clearData'),
    content: t('citationGraph.clearConfirm'),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await ClearCitationData(pid)
        graphData.value = null
        loadedOnce.value = false
        message.success(t('citationGraph.cleared'))
        loadGraph()
      } catch (e: any) {
        message.error(localizeBackendError(e, t))
      }
    },
  })
}

function onCitationProgress(data: any) {
  if (data?.type === 'paper') {
    progress.current = data.current
    progress.total = data.total
    progress.title = data.title || ''
  }
}

function onCitationDone(data: any) {
  fetching.value = false

  const reason = data?.reason as string | undefined
  const errors = (data?.errors as number | undefined) ?? 0
  const internalLinks = (data?.internal_links as number | undefined) ?? 0
  const externalPapers = (data?.external_papers as number | undefined) ?? 0

  if (reason) {
    blockReason.value = reason
  } else if (errors > 0) {
    message.warning(t('citationGraph.fetchPartialErrors', { errors }))
  } else {
    message.success(t('citationGraph.fetchComplete', { links: internalLinks, external: externalPapers }))
  }
  loadGraph()
}

async function exportPNG(): Promise<string | null> {
  const svg = (graphRef.value as any)?.$el?.querySelector?.('svg') as SVGSVGElement | undefined
  if (!svg) return null
  return svgToPngBase64(svg)
}

defineExpose({ exportPNG })

onMounted(() => {
  EventsOn('citation:progress', onCitationProgress)
  EventsOn('citation:done', onCitationDone)
  loadGraph()
})

onUnmounted(() => {
  EventsOff('citation:progress')
  EventsOff('citation:done')
})
</script>

<style scoped>
.graph-legend {
  position: absolute;
  bottom: 12px;
  left: 12px;
  background: var(--surface-overlay, rgba(0, 0, 0, 0.7));
  border-radius: var(--radius-md, 8px);
  padding: 8px 12px;
  font-size: var(--text-caption, 11px);
}

.graph-legend__item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.graph-legend__item + .graph-legend__item {
  margin-top: 4px;
}

.graph-legend__dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.graph-legend__dot--approved {
  background: var(--color-success-500, #22c55e);
}

.graph-legend__dot--downloaded {
  background: var(--color-accent-500, #8b5cf6);
}

.graph-legend__dot--external {
  background: var(--color-secondary-500, #f59e0b);
  border: 2px dashed var(--color-secondary-500, #f59e0b);
}

.graph-year-hint {
  position: absolute;
  top: 12px;
  right: 12px;
  background: var(--surface-overlay, rgba(0, 0, 0, 0.7));
  border-radius: var(--radius-md, 8px);
  padding: 6px 10px;
  font-size: 11px;
  color: var(--text-tertiary, #94a3b8);
  max-width: 220px;
}
</style>
