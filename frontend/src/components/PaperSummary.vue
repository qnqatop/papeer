<template>
  <n-card v-if="hasPDF" size="small" :title="t('papers.summary.title')" style="margin-top: 12px">
    <!-- Loading -->
    <n-spin v-if="loading" size="small" />

    <!-- No active LLM profile -->
    <n-space v-else-if="!llmStore.hasActiveProfile && !anySummary" :size="6" align="center">
      <n-text depth="3">{{ t('papers.summary.noProfile') }}</n-text>
      <n-button size="tiny" type="primary" ghost @click="goToSettings">
        {{ t('papers.summary.goToSettings') }}
      </n-button>
    </n-space>

    <!-- Existing summaries -->
    <template v-if="summaries.length > 0">
      <!-- Tabs if multiple models -->
      <n-tabs v-if="summaries.length > 1" v-model:value="activeTab" type="segment" size="small">
        <n-tab-pane v-for="s in summaries" :key="s.model" :name="s.model" :tab="s.model" />
      </n-tabs>

      <!-- Current summary -->
      <template v-for="s in summaries" :key="s.model">
        <div v-if="activeTab === s.model || summaries.length === 1" style="margin-top: 8px">
          <!-- Error state -->
          <n-alert v-if="s.status === 'error'" type="error" style="margin-bottom: 8px">
            <template #header>{{ s.error_msg }}</template>
          </n-alert>

          <!-- Markdown content -->
          <div v-if="s.status === 'done'" class="summary-content" v-html="renderMarkdown(s.content)" />

          <!-- Meta -->
          <n-space v-if="s.status === 'done'" :size="12" style="margin-top: 8px">
            <n-text depth="3" style="font-size: 12px">
              {{ s.model }} · {{ formatDate(s.updated_at || s.created_at) }}
            </n-text>
            <n-text depth="3" style="font-size: 12px">
              {{ s.tokens_in + s.tokens_out }} tokens
            </n-text>
          </n-space>

          <!-- Actions -->
          <n-space :size="8" style="margin-top: 8px">
            <n-button
              v-if="s.status === 'done' || s.status === 'error'"
              size="tiny"
              :loading="store.isGenerating(paperID, s.model)"
              @click="handleRegenerate(s)"
            >
              {{ s.status === 'error' ? t('papers.summary.retry') : t('papers.summary.regenerate') }}
            </n-button>
            <n-button size="tiny" @click="handleDelete(s)">
              {{ t('common.delete') }}
            </n-button>
            <n-button size="tiny" @click="copyToClipboard(s.content)">
              {{ t('papers.summary.copy') }}
            </n-button>
          </n-space>
        </div>
      </template>
    </template>

    <!-- No summary yet: full generate block -->
    <template v-if="!anyDone && !anyError && llmStore.hasActiveProfile">
      <n-text depth="3" style="display: block; margin-bottom: 8px">
        {{ t('papers.summary.noSummary') }}
      </n-text>
      <n-space :size="8" align="center">
        <n-text depth="3" style="font-size: 12px">{{ t('papers.summary.profile') }}: {{ llmStore.activeProfile?.name }}</n-text>
        <n-select
          v-if="modelOptions.length > 1"
          v-model:value="selectedModel"
          :options="modelOptions"
          size="small"
          style="width: 180px"
        />
        <n-button
          size="small"
          type="primary"
          :loading="anyGenerating"
          :disabled="anyGenerating || !selectedModel"
          @click="generate"
        >
          {{ t('papers.summary.generate') }}
        </n-button>
      </n-space>
    </template>

    <!-- Has summaries: add from other models -->
    <n-space
      v-if="anyDone && llmStore.hasActiveProfile && missingModels.length > 0"
      :size="8"
      align="center"
      style="margin-top: 12px; padding-top: 8px; border-top: 1px solid var(--n-border-color)"
    >
      <n-text depth="3" style="font-size: 12px">{{ t('papers.summary.addFrom') }}</n-text>
      <n-select
        v-if="missingModelOptions.length > 1"
        v-model:value="selectedModel"
        :options="missingModelOptions"
        size="tiny"
        style="width: 160px"
      />
      <n-button
        size="tiny"
        :loading="anyGenerating"
        :disabled="anyGenerating"
        @click="generateFromMissing"
      >
        {{ t('papers.summary.generate') }}
      </n-button>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NCard, NSpin, NText, NSpace, NButton, NSelect, NTabs, NTabPane, NAlert,
  useMessage, useDialog,
} from 'naive-ui'
import { useSummaryStore } from '../stores/summary'
import { useLLMProfilesStore } from '../stores/llmProfiles'
import { useFirstVisitHint } from '../composables/useFirstVisitHint'
import { renderMarkdown } from '../utils/markdown'
import { db } from '../../wailsjs/go/models'

const props = defineProps<{
  paperID: number
  hasPDF: boolean
}>()

const { t } = useI18n()
const store = useSummaryStore()
const llmStore = useLLMProfilesStore()
const message = useMessage()
const dialog = useDialog()
const router = useRouter()

function goToSettings() {
  router.push({ name: 'settings', hash: '#ai' })
}

useFirstVisitHint('paperSummary', {
  steps: () => [{
    popover: { title: t('hints.paperSummary.title'), description: t('hints.paperSummary.description') },
  }],
  ready: () => props.hasPDF,
})

const loading = ref(false)
const selectedModel = ref('')
const activeTab = ref('')

const summaries = computed(() =>
  store.summariesByPaper.get(props.paperID) || [],
)

const availableModels = computed(() => llmStore.activeProfile?.models || [])

const modelOptions = computed(() =>
  availableModels.value.map(m => ({
    label: m === llmStore.activeProfile?.default_model ? `★ ${m}` : m,
    value: m,
  })),
)

const anyGenerating = computed(() =>
  summaries.value.some(s => s.status === 'generating') ||
  availableModels.value.some(m => store.isGenerating(props.paperID, m)),
)

const anySummary = computed(() => summaries.value.length > 0)
const anyDone = computed(() => summaries.value.some(s => s.status === 'done'))
const anyError = computed(() => summaries.value.some(s => s.status === 'error'))

const existingModels = computed(() => summaries.value.map(s => s.model))

const missingModels = computed(() =>
  availableModels.value.filter(m => !existingModels.value.includes(m)),
)

const missingModelOptions = computed(() =>
  missingModels.value.map(m => ({
    label: m === llmStore.activeProfile?.default_model ? `★ ${m}` : m,
    value: m,
  })),
)

function formatDate(d: string) {
  if (!d) return ''
  return new Date(d).toLocaleDateString()
}

function defaultModel(): string {
  return llmStore.activeProfile?.default_model || availableModels.value[0] || ''
}

async function generate() {
  const model = selectedModel.value || defaultModel()
  if (!model) return
  try {
    await store.generate(props.paperID, model)
  } catch (e: any) {
    message.error(e?.message || String(e))
  }
}

async function generateFromMissing() {
  const model = selectedModel.value && missingModels.value.includes(selectedModel.value)
    ? selectedModel.value
    : missingModels.value[0]
  if (!model) return
  try {
    await store.generate(props.paperID, model)
  } catch (e: any) {
    message.error(e?.message || String(e))
  }
}

function handleRegenerate(s: db.Summary) {
  dialog.warning({
    title: t('papers.summary.regenerateConfirmTitle'),
    content: t('papers.summary.regenerateConfirm', { model: s.model }),
    positiveText: t('papers.summary.regenerate'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await store.regenerate(props.paperID, s.model)
        message.success(t('papers.summary.regenerated'))
      } catch (e: any) {
        message.error(e?.message || String(e))
      }
    },
  })
}

function handleDelete(s: db.Summary) {
  dialog.warning({
    title: t('papers.summary.deleteConfirmTitle'),
    content: t('papers.summary.deleteConfirm'),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await store.remove(s.id, props.paperID)
        message.success(t('papers.summary.deleted'))
      } catch (e: any) {
        message.error(e?.message || String(e))
      }
    },
  })
}

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success(t('papers.summary.copied'))
  } catch {
    message.error(t('papers.summary.copyFailed'))
  }
}

onMounted(async () => {
  if (!props.hasPDF) return
  loading.value = true
  try {
    if (llmStore.profiles.length === 0) await llmStore.fetch()
    await store.fetchForPaper(props.paperID)
    const list = store.summariesByPaper.get(props.paperID) || []
    if (list.length > 0) {
      activeTab.value = list[0].model
    }
    selectedModel.value = missingModels.value[0] || defaultModel()
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.summary-content {
  line-height: 1.7;
  font-size: 14px;
}
.summary-content :deep(h2) { font-size: 18px; margin: 12px 0 6px; }
.summary-content :deep(h3) { font-size: 16px; margin: 10px 0 4px; }
.summary-content :deep(h4) { font-size: 14px; margin: 8px 0 4px; }
.summary-content :deep(ul) { margin: 4px 0; padding-left: 20px; }
.summary-content :deep(li) { margin: 2px 0; }
.summary-content :deep(p) { margin: 6px 0; }
.summary-content :deep(strong) { font-weight: 600; }
.summary-content :deep(table) { border-collapse: collapse; width: 100%; margin: 8px 0; font-size: 13px; }
.summary-content :deep(th) { background: var(--surface-border, rgba(148, 163, 184, 0.12)); text-align: left; padding: 6px 8px; border: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08)); font-weight: 600; }
.summary-content :deep(td) { padding: 5px 8px; border: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08)); }
.summary-content :deep(tr:nth-child(even)) { background: rgba(148, 163, 184, 0.04); }
.summary-content :deep(.math-block) { display: block; text-align: center; margin: 12px 0; font-style: italic; }
.summary-content :deep(.math-inline) { font-family: serif; font-style: italic; background: rgba(148, 163, 184, 0.06); padding: 1px 4px; border-radius: 3px; font-size: 0.95em; }
</style>
