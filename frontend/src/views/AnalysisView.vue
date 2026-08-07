<template>
  <n-card>
    <template v-if="!profileStore.activeProfileId">
      <n-empty :description="t('common.selectProfileFirst')" />
    </template>

    <template v-else>
      <!-- Compact status line: one line instead of 3 hero plaques + Papers by Status -->
      <n-space align="center" justify="space-between" style="margin-bottom: 16px" :wrap="true">
        <n-text style="font-size: 14px">
          <strong>{{ statusLine }}</strong>
        </n-text>
        <n-space align="center" :size="12">
          <n-tag v-if="pdfAvailability" size="small" :type="pdfAvailability.percent >= 70 ? 'success' : 'warning'">
            {{ t('v2.analysis.pdfAvailability', { percent: Math.round(pdfAvailability.percent) }) }}
          </n-tag>
          <n-button size="small" :loading="exportingBundle" @click="exportBundle">
            {{ t('v2.analysis.exportBundle') }}
          </n-button>
        </n-space>
      </n-space>

      <n-tabs v-model:value="activeTab" type="line" display-directive="show:lazy">
        <n-tab-pane :name="'overview'" :tab="t('v2.analysis.tabOverview')">
          <AnalysisOverviewTab />
        </n-tab-pane>

        <n-tab-pane :name="'coverage-gaps'" :tab="t('v2.analysis.tabCoverageGaps')">
          <CoverageGapsTab />
        </n-tab-pane>

        <n-tab-pane :name="'topics'" :tab="t('v2.analysis.tabTopics')">
          <TopicsTab />
        </n-tab-pane>

        <n-tab-pane :name="'key-papers'" :tab="t('v2.analysis.tabKeyPapers')">
          <KeyPapersTab />
        </n-tab-pane>

        <n-tab-pane :name="'graph'" :tab="t('v2.analysis.tabGraph')">
          <CitationGraphView ref="graphViewRef" @switch-tab="activeTab = $event" />
        </n-tab-pane>

        <n-tab-pane :name="'review'" :tab="t('v2.analysis.tabReview')">
          <ReviewDraftTab v-model:markdown="reviewMarkdown" />
        </n-tab-pane>

        <n-tab-pane :name="'recs'" :tab="t('v2.analysis.tabRecs')">
          <RecsPanel :profile-id="profileStore.activeProfileId!" @open-detail="openInPapers" @status-changed="loadHeaderStats" />
        </n-tab-pane>
      </n-tabs>
    </template>
  </n-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NCard, NEmpty, NTabs, NTabPane, NText, NTag, NSpace, NButton,
  useMessage,
} from 'naive-ui'
import { useOnboardingPhase } from '../composables/useOnboarding'
import { useProfileStore } from '../stores/profile'
import { GetStats, GetPDFAvailability, SaveAnalysisBundle } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'
import { localizeBackendError } from '../utils/errors'
import AnalysisOverviewTab from './AnalysisOverviewTab.vue'
import CoverageGapsTab from './CoverageGapsTab.vue'
import TopicsTab from './TopicsTab.vue'
import KeyPapersTab from './KeyPapersTab.vue'
import CitationGraphView from './CitationGraphView.vue'
import ReviewDraftTab from './ReviewDraftTab.vue'
import RecsPanel from './RecsPanel.vue'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const profileStore = useProfileStore()

const activeTab = ref('overview')
const totalCount = ref(0)
const processedCount = ref(0)
const downloadedCount = ref(0)
const pdfAvailability = ref<db.PDFAvailability | null>(null)
const reviewMarkdown = ref('')
const graphViewRef = ref<InstanceType<typeof CitationGraphView> | null>(null)
const exportingBundle = ref(false)

// Onboarding phase 5: analysis
useOnboardingPhase(5, {
  precondition: () => totalCount.value > 0,
  getSteps: () => [{
    popover: {
      title: t('onboarding.analysis.title'),
      description: t('onboarding.analysis.description'),
    },
  }],
})

const statusLine = computed(() =>
  t('v2.analysis.statusLine', {
    total: totalCount.value,
    processed: processedCount.value,
    downloaded: downloadedCount.value,
  })
)

function openInPapers(paper: db.Paper) {
  router.push({ path: '/papers', query: { paper_id: String(paper.id) } })
}

async function loadHeaderStats() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    const stats = await GetStats(pid, null)
    const pbs = stats.papers_by_status ?? {}
    const newC = pbs['new'] ?? 0
    const approved = pbs['approved'] ?? 0
    const rejected = pbs['rejected'] ?? 0
    const downloaded = pbs['downloaded'] ?? 0
    totalCount.value = newC + approved + rejected + downloaded
    processedCount.value = approved + rejected + downloaded
    downloadedCount.value = downloaded
  } catch { /* ignore */ }

  try {
    pdfAvailability.value = await GetPDFAvailability(pid)
  } catch {
    pdfAvailability.value = null
  }
}

async function exportBundle() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  exportingBundle.value = true
  try {
    let pngBase64 = ''
    if (graphViewRef.value) {
      pngBase64 = (await graphViewRef.value.exportPNG()) || ''
    }
    const path = await SaveAnalysisBundle(pid, reviewMarkdown.value, pngBase64)
    if (path) message.success(t('v2.analysis.bundleSaved', { path }))
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
  } finally {
    exportingBundle.value = false
  }
}

onMounted(() => {
  loadHeaderStats()
})

watch(() => profileStore.activeProfileId, () => {
  reviewMarkdown.value = ''
  loadHeaderStats()
})
</script>
