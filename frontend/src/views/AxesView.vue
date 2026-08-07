<template>
  <n-card :title="t('v2.topics.title')">
    <template v-if="!profileStore.activeProfile">
      <n-empty :description="t('common.selectProfileFirst')" />
    </template>

    <template v-else>
      <!-- Top bar -->
      <n-space justify="space-between" align="center" style="margin-bottom: 16px">
        <n-space align="center">
          <n-button type="primary" @click="addAxis" data-onboarding="add-axis">{{ t('v2.topics.addTopic') }}</n-button>
          <n-button
            type="info"
            :disabled="progressStore.searching || axes.length === 0"
            :loading="progressStore.searching"
            @click="showLogDrawer = true; searchAll()"
            data-onboarding="search-all"
          >
            {{ t('axes.searchAll') }}
          </n-button>
          <n-button
            v-if="progressStore.searching"
            type="error"
            size="small"
            @click="cancelSearch"
          >
            {{ t('common.cancel') }}
          </n-button>
        </n-space>
        <n-space :size="8">
          <n-button quaternary @click="showLogDrawer = true" :disabled="progressStore.searchEvents.length === 0">
            {{ t('v2.searchLog.expandLog') }}
          </n-button>
          <n-dropdown :options="moreMenuOptions" @select="handleMoreMenu">
            <n-button quaternary>&#8943;</n-button>
          </n-dropdown>
        </n-space>
      </n-space>

      <!-- Warnings -->
      <n-alert v-if="!profileStore.activeProfile?.email" type="warning" style="margin-bottom: 12px" :show-icon="true">
        {{ t('axes.warnNoEmail') }}
      </n-alert>
      <!-- Progress -->
      <n-space v-if="progressStore.searching || progressStore.lastEvent" align="center" style="margin-bottom: 12px">
        <n-spin v-if="progressStore.searching" size="small" />
        <span style="font-size: 13px; color: var(--n-text-color-3)">{{ progressStore.lastEvent }}</span>
      </n-space>

      <!-- Loading -->
      <n-spin :show="loading">
        <n-collapse v-if="axes.length > 0" accordion data-onboarding="axes-list">
          <n-collapse-item
            v-for="axis in axes"
            :key="axis.id"
            :name="axis.id"
          >
            <template #header>
              <n-space align="center" :size="8">
                <span style="font-weight: 600">{{ axis.axis_key }}</span>
                <n-badge
                  :value="paperCounts[axis.id] ?? 0"
                  :max="9999"
                  type="info"
                  show-zero
                />
              </n-space>
            </template>

            <template #header-extra>
              <n-space :size="4" @click.stop>
                <n-button
                  size="small"
                  type="info"
                  :disabled="progressStore.searching"
                  :loading="searchingAxisId === axis.id"
                  @click="searchOne(axis)"
                >
                  {{ t('common.search') }}
                </n-button>
                <n-button size="small" type="primary" quaternary @click.stop="exportAxis(axis.id)">
                  {{ t('axes.exportYaml') }}
                </n-button>
                <n-popconfirm @positive-click="removeAxis(axis.id)">
                  <template #trigger>
                    <n-button size="small" type="error" quaternary>{{ t('common.delete') }}</n-button>
                  </template>
                  {{ t('axes.deleteConfirm', { key: axis.axis_key }) }}
                </n-popconfirm>
              </n-space>
            </template>

            <!-- Axis details -->
            <n-space vertical :size="16" style="padding: 4px 0">
              <!-- Key & description -->
              <n-space vertical :size="8">
                <n-input
                  v-model:value="axis.axis_key"
                  :placeholder="t('axes.axisKeyPlaceholder')"
                  style="max-width: 300px"
                />
                <n-input
                  v-model:value="axis.description"
                  type="textarea"
                  :placeholder="t('axes.descriptionPlaceholder')"
                  :autosize="{ minRows: 1, maxRows: 4 }"
                />
              </n-space>

              <!-- year_min -->
              <n-space align="center" :size="4">
                <span style="font-size: 13px">{{ t('axes.yearMin') }}</span>
                <n-input-number
                  v-model:value="axis.year_min"
                  :min="1900"
                  :max="2030"
                  clearable
                  size="small"
                  style="width: 120px"
                />
              </n-space>

              <!-- Queries -->
              <n-card size="small" :title="t('axes.queries')" embedded>
                <n-space vertical :size="8">
                  <n-space
                    v-for="(q, qi) in axis.queries"
                    :key="qi"
                    align="center"
                    :size="4"
                  >
                    <n-input
                      v-model:value="q.text"
                      :placeholder="t('axes.searchQueryPlaceholder')"
                      style="min-width: 400px"
                    />
                    <n-button
                      size="small"
                      quaternary
                      type="error"
                      @click="axis.queries.splice(qi, 1)"
                    >
                      {{ t('common.remove') }}
                    </n-button>
                  </n-space>
                  <n-button size="small" dashed @click="addQuery(axis)">
                    {{ t('axes.addQuery') }}
                  </n-button>
                </n-space>
              </n-card>

              <!-- Keywords -->
              <n-card size="small" :title="t('axes.keywords')" embedded>
                <n-space vertical :size="12">
                  <!-- Must keywords -->
                  <n-space vertical :size="4">
                    <n-space align="center" :size="4">
                      <span style="font-size: 13px; font-weight: 500">{{ t('axes.mustRequired') }}</span>
                      <n-tooltip>
                        <template #trigger>
                          <n-icon :component="HelpCircleOutline" size="14" style="cursor: help; opacity: 0.5" />
                        </template>
                        {{ t('v2.topics.mustTooltip') }}
                      </n-tooltip>
                    </n-space>
                    <n-space :size="4" :wrap="true">
                      <n-tag
                        v-for="(kw, ki) in mustKeywords(axis)"
                        :key="ki"
                        type="error"
                        closable
                        @close="removeKeyword(axis, kw)"
                      >
                        {{ kw.word }}
                      </n-tag>
                    </n-space>
                    <n-input
                      size="small"
                      :placeholder="t('axes.addMustPlaceholder')"
                      style="max-width: 250px"
                      @keydown.enter="(e: KeyboardEvent) => addKeyword(axis, 'must', e)"
                    />
                    <span style="font-size: 11px; color: var(--n-text-color-3)">
                      {{ t('axes.keywordSplitHint') }}
                    </span>
                  </n-space>

                  <!-- Boost keywords -->
                  <n-space vertical :size="4">
                    <n-space align="center" :size="4">
                      <span style="font-size: 13px; font-weight: 500">{{ t('axes.boostRanking') }}</span>
                      <n-tooltip>
                        <template #trigger>
                          <n-icon :component="HelpCircleOutline" size="14" style="cursor: help; opacity: 0.5" />
                        </template>
                        {{ t('v2.topics.boostTooltip') }}
                      </n-tooltip>
                    </n-space>
                    <n-space :size="4" :wrap="true">
                      <n-tag
                        v-for="(kw, ki) in boostKeywords(axis)"
                        :key="ki"
                        type="info"
                        closable
                        @close="removeKeyword(axis, kw)"
                      >
                        {{ kw.word }}
                      </n-tag>
                    </n-space>
                    <n-input
                      size="small"
                      :placeholder="t('axes.addBoostPlaceholder')"
                      style="max-width: 250px"
                      @keydown.enter="(e: KeyboardEvent) => addKeyword(axis, 'boost', e)"
                    />
                    <span style="font-size: 11px; color: var(--n-text-color-3)">
                      {{ t('axes.keywordSplitHint') }}
                    </span>
                  </n-space>

                  <!-- Exclude keywords -->
                  <n-space vertical :size="4">
                    <n-space align="center" :size="4">
                      <span style="font-size: 13px; font-weight: 500">{{ t('axes.excludeTitle') }}</span>
                      <n-tooltip>
                        <template #trigger>
                          <n-icon :component="HelpCircleOutline" size="14" style="cursor: help; opacity: 0.5" />
                        </template>
                        {{ t('axes.excludeTooltip') }}
                      </n-tooltip>
                    </n-space>
                    <n-space :size="4" :wrap="true">
                      <n-tag
                        v-for="(kw, ki) in excludeKeywords(axis)"
                        :key="ki"
                        type="warning"
                        closable
                        @close="removeKeyword(axis, kw)"
                      >
                        {{ kw.word }}
                      </n-tag>
                    </n-space>
                    <n-input
                      size="small"
                      :placeholder="t('axes.addExcludePlaceholder')"
                      style="max-width: 250px"
                      @keydown.enter="(e: KeyboardEvent) => addKeyword(axis, 'exclude', e)"
                    />
                    <span style="font-size: 11px; color: var(--n-text-color-3)">
                      {{ t('axes.keywordSplitHint') }}
                    </span>
                  </n-space>
                </n-space>
              </n-card>

              <!-- Save -->
              <n-button type="primary" :loading="savingAxisId === axis.id" @click="saveAxis(axis)">
                {{ t('common.save') }}
              </n-button>
            </n-space>
          </n-collapse-item>
        </n-collapse>

        <!-- Empty state -->
        <div v-else class="axes-empty">
          <svg width="56" height="56" viewBox="0 0 56 56" fill="none" xmlns="http://www.w3.org/2000/svg" class="axes-empty__icon">
            <circle cx="24" cy="24" r="18" stroke="var(--color-primary-400)" stroke-width="2" opacity="0.4" />
            <line x1="37" y1="37" x2="50" y2="50" stroke="var(--color-primary-400)" stroke-width="2.5" stroke-linecap="round" opacity="0.4" />
            <line x1="18" y1="24" x2="30" y2="24" stroke="var(--color-primary-400)" stroke-width="2" stroke-linecap="round" />
            <line x1="24" y1="18" x2="24" y2="30" stroke="var(--color-primary-400)" stroke-width="2" stroke-linecap="round" />
          </svg>
          <h3 class="axes-empty__title">{{ t('v2.topics.emptyTitle') }}</h3>
          <p class="axes-empty__description">{{ t('v2.topics.emptyDescription') }}</p>
          <n-space justify="center" :size="12">
            <n-button type="primary" @click="addAxis">{{ t('v2.topics.addTopic') }}</n-button>
            <n-button @click="importYaml">{{ t('axes.importYaml') }}</n-button>
          </n-space>
        </div>
      </n-spin>
    </template>
  </n-card>

  <!-- Search log drawer -->
  <SearchLogDrawer v-model:show="showLogDrawer" />

  <!-- Search progress bar -->
  <SearchProgressBar @open-log="showLogDrawer = true" @cancel="cancelSearch" />
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { isInvalidEmailError } from '../utils/errors'
import { useOnboardingPhase } from '../composables/useOnboarding'
import { useOnboardingStore } from '../stores/onboarding'
import {
  NCard, NEmpty, NSpace, NButton, NInput, NInputNumber, NIcon,
  NTag, NBadge, NCollapse, NCollapseItem, NPopconfirm, NSpin, NAlert,
  NDropdown, NTooltip, NH3, NText,
  useMessage,
} from 'naive-ui'
import { HelpCircleOutline } from '@vicons/ionicons5'
import SearchLogDrawer from '../components/SearchLogDrawer.vue'
import SearchProgressBar from '../components/SearchProgressBar.vue'
import { useProfileStore } from '../stores/profile'
import { useProgressStore } from '../stores/progress'
import {
  ListAxes,
  SaveAxis,
  DeleteAxis,
  SearchAxis,
  SearchAllAxes,
  CountPapersByAxis,
  CancelOperation,
  ImportYAML,
  ExportAxisYAML,
  ExportAllAxesYAML,
} from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

const { t } = useI18n()
const vueRouter = useRouter()
const route = useRoute()
const profileStore = useProfileStore()
const progressStore = useProgressStore()
const onboardingStore = useOnboardingStore()
const message = useMessage()

const showLogDrawer = ref(false)

// Open log drawer if coming from redirect with ?log=open
watch(() => route.query.log, (val) => {
  if (val === 'open') {
    showLogDrawer.value = true
  }
}, { immediate: true })

// "⋯" more menu
const moreMenuOptions = computed(() => [
  { label: t('axes.importYaml'), key: 'import' },
  { label: t('axes.exportAllYaml'), key: 'export', disabled: axes.value.length === 0 },
])

function handleMoreMenu(key: string) {
  if (key === 'import') importYaml()
  else if (key === 'export') exportAllAxes()
}

const axes = ref<db.Axis[]>([])
const paperCounts = ref<Record<number, number>>({})
const loading = ref(false)
const savingAxisId = ref<number | null>(null)
const searchingAxisId = ref<number | null>(null)

// Onboarding phase 2: explain topics (demo) or prompt to add first axis (normal)
useOnboardingPhase(2, {
  precondition: () => !loading.value && !!profileStore.activeProfile &&
    (onboardingStore.isDemoMode ? axes.value.length > 0 : axes.value.length === 0),
  getSteps: () => {
    if (onboardingStore.isDemoMode) {
      // Demo mode: explain the pre-created topic step-by-step
      return [
        {
          popover: {
            title: t('onboarding.demo.whySearch'),
            description: t('onboarding.demo.whySearchDescription'),
          },
        },
        {
          element: '[data-onboarding="axes-list"]',
          popover: {
            title: t('onboarding.demo.topicCreated'),
            description: t('onboarding.demo.topicCreatedDescription'),
          },
        },
        {
          element: '[data-onboarding="search-all"]',
          popover: {
            title: t('onboarding.demo.readyToSearch'),
            description: t('onboarding.demo.readyToSearchDescription'),
          },
        },
      ]
    }
    // Normal mode: prompt to add first axis
    return [{
      element: '[data-onboarding="add-axis"]',
      popover: {
        title: t('onboarding.axes.addTitle'),
        description: t('onboarding.axes.addDescription'),
      },
    }]
  },
  onComplete: () => {
    // In demo mode, auto-trigger search after the tour explains everything
    if (onboardingStore.isDemoMode && !progressStore.searching) {
      onboardingStore.markPhaseComplete(3)
      searchAll()
    }
  },
})

// Onboarding phase 3: run search (after axes exist, non-demo only)
useOnboardingPhase(3, {
  precondition: () => !loading.value && axes.value.length > 0 &&
    !onboardingStore.shouldShowPhase(2) && !onboardingStore.isDemoMode,
  getSteps: () => [{
    element: '[data-onboarding="search-all"]',
    popover: {
      title: t('onboarding.search.title'),
      description: t('onboarding.search.description'),
    },
  }],
})

async function exportAxis(id: number) {
  try {
    await ExportAxisYAML(id)
  } catch (e: any) {
    message.error(t('axes.exportYamlFailed', { error: e }))
  }
}

async function exportAllAxes() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    await ExportAllAxesYAML(pid)
  } catch (e: any) {
    message.error(t('axes.exportYamlFailed', { error: e }))
  }
}

async function fetchAxes() {
  const pid = profileStore.activeProfileId
  if (!pid) {
    axes.value = []
    paperCounts.value = {}
    return
  }
  loading.value = true
  try {
    const [axList, counts] = await Promise.all([
      ListAxes(pid),
      CountPapersByAxis(pid),
    ])
    axes.value = axList || []
    paperCounts.value = counts || {}
  } catch (e: any) {
    message.error(t('axes.loadFailed', { error: e }))
  } finally {
    loading.value = false
  }
}

async function addAxis() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  const next = axes.value.length + 1
  const newAxis = new db.Axis({
    id: 0,
    profile_id: pid,
    axis_key: `axis-${next}`,
    description: '',
    position: next,
    queries: [],
    keywords: [],
  })
  try {
    const saved = await SaveAxis(newAxis)
    axes.value.push(saved)
  } catch (e: any) {
    message.error(t('axes.createFailed', { error: e }))
  }
}

async function saveAxis(axis: db.Axis) {
  savingAxisId.value = axis.id
  try {
    const saved = await SaveAxis(axis)
    const idx = axes.value.findIndex(a => a.id === axis.id)
    if (idx !== -1) {
      axes.value[idx] = saved
    }
    message.success(t('axes.saved'))
  } catch (e: any) {
    message.error(t('axes.saveFailed', { error: e }))
  } finally {
    savingAxisId.value = null
  }
}

async function removeAxis(id: number) {
  try {
    await DeleteAxis(id)
    axes.value = axes.value.filter(a => a.id !== id)
    message.success(t('axes.deleted'))
  } catch (e: any) {
    message.error(t('axes.deleteFailed', { error: e }))
  }
}

function addQuery(axis: db.Axis) {
  if (!axis.queries) axis.queries = []
  axis.queries.push(new db.Query({
    id: 0,
    axis_id: axis.id,
    text: '',
    position: axis.queries.length,
  }))
}

function mustKeywords(axis: db.Axis): db.Keyword[] {
  return (axis.keywords || []).filter(k => k.type === 'must')
}

function boostKeywords(axis: db.Axis): db.Keyword[] {
  return (axis.keywords || []).filter(k => k.type === 'boost')
}

function excludeKeywords(axis: db.Axis): db.Keyword[] {
  return (axis.keywords || []).filter(k => k.type === 'exclude')
}

function addKeyword(axis: db.Axis, type: string, e: KeyboardEvent) {
  const input = e.target as HTMLInputElement
  const raw = input.value.trim()
  if (!raw) return
  if (!axis.keywords) axis.keywords = []
  // Split on commas / whitespace so `lead zinc copper sediments` becomes 4 tags
  // instead of a single multi-word entry that won't match anything.
  const existing = new Set(axis.keywords.filter(k => k.type === type).map(k => k.word.toLowerCase()))
  for (const word of raw.split(/[\s,]+/)) {
    const w = word.trim()
    if (!w || existing.has(w.toLowerCase())) continue
    existing.add(w.toLowerCase())
    axis.keywords.push(new db.Keyword({
      id: 0,
      axis_id: axis.id,
      word: w,
      type,
    }))
  }
  input.value = ''
}

function removeKeyword(axis: db.Axis, kw: db.Keyword) {
  axis.keywords = (axis.keywords || []).filter(k => k !== kw)
}

async function searchOne(axis: db.Axis) {
  const pid = profileStore.activeProfileId
  if (!pid) return
  progressStore.resetSearch()
  progressStore.searching = true
  searchingAxisId.value = axis.id
  try {
    const count = await SearchAxis(pid, axis.id)
    if (count === 0 && allProvidersTimedOut()) {
      message.error(t('axes.networkUnreachable'), { duration: 10000 })
    } else {
      message.success(t('axes.foundForAxis', { count, key: axis.axis_key }))
    }
    await refreshCounts()
  } catch (e: any) {
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(t('axes.searchFailed', { error: e }))
  } finally {
    progressStore.searching = false
    searchingAxisId.value = null
  }
}

async function searchAll() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  progressStore.resetSearch()
  progressStore.searching = true
  try {
    const count = await SearchAllAxes(pid)
    if (count === 0 && allProvidersTimedOut()) {
      message.error(t('axes.networkUnreachable'), { duration: 10000 })
    } else {
      message.success(t('axes.foundTotal', { count }))
    }
    onboardingStore.markPhaseComplete(3)
    await refreshCounts()
    // In demo mode, navigate to papers after search completes
    if (onboardingStore.isDemoMode && count > 0) {
      vueRouter.push({ name: 'papers' })
    }
  } catch (e: any) {
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(t('axes.searchFailed', { error: e }))
  } finally {
    progressStore.searching = false
  }
}

// allProvidersTimedOut returns true when the search produced no papers AND
// the majority of provider errors look like network timeouts. Lets us surface
// a "configure VPN/proxy" hint instead of the generic "0 papers" message.
function allProvidersTimedOut(): boolean {
  const errors = progressStore.searchEvents.filter(e => e.type === 'provider_error')
  if (errors.length < 3) return false
  const timeouts = errors.filter(e => /context deadline exceeded|timeout|no such host|connection refused/i.test(e.error || ''))
  return timeouts.length >= Math.ceil(errors.length * 0.75)
}

async function cancelSearch() {
  try {
    await CancelOperation()
  } catch (e: any) {
    message.error(t('axes.cancelFailed', { error: e }))
  }
}

async function refreshCounts() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    paperCounts.value = await CountPapersByAxis(pid) || {}
  } catch {
    // ignore
  }
}

async function importYaml() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    const count = await ImportYAML(pid)
    if (count > 0) {
      message.success(t('axes.imported', { count }))
      await fetchAxes()
    }
  } catch (e: any) {
    message.error(t('axes.importFailed', { error: e }))
  }
}

async function createDemoAxis() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    const saved = await SaveAxis(new db.Axis({
      id: 0,
      profile_id: pid,
      axis_key: 'machine-learning',
      description: 'Machine learning methods and applications',
      position: 1,
      queries: [
        new db.Query({ id: 0, axis_id: 0, text: 'deep learning survey', position: 0 }),
        new db.Query({ id: 0, axis_id: 0, text: 'neural network optimization methods', position: 1 }),
      ],
      keywords: [
        new db.Keyword({ id: 0, axis_id: 0, word: 'machine learning', type: 'must' }),
        new db.Keyword({ id: 0, axis_id: 0, word: 'deep learning', type: 'boost' }),
        new db.Keyword({ id: 0, axis_id: 0, word: 'neural network', type: 'boost' }),
        new db.Keyword({ id: 0, axis_id: 0, word: 'optimization', type: 'boost' }),
      ],
    }))
    axes.value = [saved]
  } catch {
    // ignore — onboarding will just not trigger
  }
}

onMounted(async () => {
  await fetchAxes()
  // In demo mode, create the demo axis here so it appears on screen before the tour starts
  if (onboardingStore.isDemoMode && axes.value.length === 0 && onboardingStore.shouldShowPhase(2)) {
    await createDemoAxis()
  }
})

watch(() => profileStore.activeProfileId, () => {
  fetchAxes()
})
</script>

<style scoped>
.axes-empty {
  text-align: center;
  padding: var(--space-16, 64px) var(--space-5, 20px);
}

.axes-empty__icon {
  margin-bottom: var(--space-5, 20px);
}

.axes-empty__title {
  font-size: var(--text-h2, 22px);
  font-weight: var(--weight-semibold, 600);
  margin: 0 0 var(--space-2, 8px) 0;
  color: var(--text-primary, #f8fafc);
}

.axes-empty__description {
  font-size: var(--text-body, 15px);
  color: var(--text-tertiary, #94a3b8);
  margin: 0 0 var(--space-6, 24px) 0;
  max-width: 480px;
  margin-left: auto;
  margin-right: auto;
  line-height: var(--leading-normal, 1.6);
}

/* Collapse item styling */
:deep(.n-collapse-item) {
  border-radius: var(--radius-md, 8px);
  margin-bottom: var(--space-2, 8px);
}

:deep(.n-collapse-item__header) {
  padding: var(--space-3, 12px) var(--space-4, 16px) !important;
}

:deep(.n-collapse-item__content-inner) {
  padding: var(--space-2, 8px) var(--space-4, 16px) var(--space-4, 16px) !important;
}
</style>
