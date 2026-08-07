<template>
  <n-space vertical :size="16">
    <n-card v-if="!profileStore.activeProfileId" :title="t('reviewDraft.title')">
      <n-empty :description="t('common.selectProfileFirst')" />
    </n-card>

    <template v-else>
      <!-- No LLM profile configured -->
      <n-card v-if="!llmStore.hasActiveProfile" :title="t('reviewDraft.title')">
        <n-space :size="8" align="center">
          <n-text depth="3">{{ t('reviewDraft.noLlmProfile') }}</n-text>
          <n-button size="small" type="primary" ghost @click="router.push('/settings')">
            {{ t('reviewDraft.goToSettings') }}
          </n-button>
        </n-space>
      </n-card>

      <template v-else>
        <!-- Paper selection -->
        <n-card v-if="downloadedPapers.length > 0" :title="t('reviewDraft.paperSelection')">
          <n-space vertical :size="8">
            <n-space :size="16">
              <n-radio :checked="selectAll" :value="true" @update:checked="selectAll = true">
                {{ t('reviewDraft.allDownloaded', { count: downloadedPapers.length }) }}
              </n-radio>
              <n-radio :checked="!selectAll" :value="false" @update:checked="selectAll = false">
                {{ t('reviewDraft.selectSpecific') }}
              </n-radio>
            </n-space>

            <div v-if="!selectAll" style="max-height: 240px; overflow-y: auto; padding-top: 4px">
              <n-checkbox
                v-for="paper in downloadedPapers"
                :key="paper.id"
                :checked="selectedPaperIDs.includes(paper.id)"
                @update:checked="(checked: boolean) => {
                  if (checked) selectedPaperIDs.push(paper.id)
                  else selectedPaperIDs = selectedPaperIDs.filter(id => id !== paper.id)
                }"
                style="display: block; margin-bottom: 4px"
              >
                <n-text style="font-size: 13px">{{ paper.title }} {{ paper.year ? '(' + paper.year + ')' : '' }}</n-text>
              </n-checkbox>
            </div>
          </n-space>
        </n-card>

        <n-card v-else>
          <n-empty :description="t('reviewDraft.noDownloadedPapers')" />
        </n-card>

        <!-- Controls -->
        <n-card>
          <n-space align="center" :wrap="true" :size="12">
            <n-text depth="3" style="font-size: 13px">{{ t('reviewDraft.model') }}:</n-text>
            <n-select v-model:value="selectedModel" :options="modelOptions" size="small" style="width: 240px" />
            <n-button type="primary" size="small" :loading="generating" :disabled="generating" @click="generate">
              {{ t('reviewDraft.generate') }}
            </n-button>
            <n-button v-if="generating" size="small" quaternary @click="cancel">
              {{ t('common.cancel') }}
            </n-button>
            <n-button v-if="markdown" size="small" @click="save">
              {{ t('reviewDraft.saveMd') }}
            </n-button>
          </n-space>

          <div style="margin-top: 8px">
            <n-text
              depth="3"
              style="cursor: pointer; font-size: 12px"
              @click="showCustomPrompt = !showCustomPrompt"
            >
              {{ t('reviewDraft.customPromptToggle') }}
            </n-text>
            <n-space v-if="showCustomPrompt" vertical :size="6" style="margin-top: 8px">
              <n-input
                v-model:value="customPrompt"
                type="textarea"
                :placeholder="t('reviewDraft.customPromptPlaceholder')"
                :autosize="{ minRows: 3, maxRows: 8 }"
              />
              <n-text depth="3" style="font-size: 11px">{{ t('reviewDraft.customPromptHint') }}</n-text>
            </n-space>
          </div>

          <!-- Progress -->
          <n-space v-if="generating" vertical :size="8" style="margin-top: 12px">
            <n-progress type="line" :percentage="progressPercent" :height="6" :processing="true" />
            <n-text depth="3" style="font-size: 12px">
              {{ t('reviewDraft.progressLabel', { current: progress.current, total: progress.total, label: progress.label }) }}
            </n-text>
          </n-space>
        </n-card>

        <!-- Preview -->
        <n-card v-if="markdown" :title="t('reviewDraft.previewTitle')">
          <n-alert type="warning" style="margin-bottom: 12px">
            {{ t('reviewDraft.hallucinationWarning') }}
          </n-alert>
          <div class="review-content" v-html="renderMarkdown(markdown)" />
        </n-card>

        <!-- Empty (never generated) -->
        <n-card v-else-if="!generating">
          <n-empty :description="t('reviewDraft.empty')" />
        </n-card>
      </template>
    </template>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NCard, NEmpty, NSpace, NText, NSelect, NButton, NProgress, NAlert, NInput,
  NCheckbox, NRadio,
  useMessage,
} from 'naive-ui'
import { GenerateReviewDraft, GetDownloadedPapers, CancelOperation, SaveExportFile } from '../../wailsjs/go/app/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { app, db } from '../../wailsjs/go/models'
import { useProfileStore } from '../stores/profile'
import { useLLMProfilesStore } from '../stores/llmProfiles'
import { localizeBackendError, isInvalidEmailError } from '../utils/errors'
import { renderMarkdown } from '../utils/markdown'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const profileStore = useProfileStore()
const llmStore = useLLMProfilesStore()

const markdown = defineModel<string>('markdown', { default: '' })

const generating = ref(false)
const selectedModel = ref<string | null>(null)
const customPrompt = ref('')
const showCustomPrompt = ref(false)
const downloadedPapers = ref<db.Paper[]>([])
const selectedPaperIDs = ref<number[]>([])
const selectAll = ref(true)
const progress = reactive({ current: 0, total: 0, label: '' })

const progressPercent = computed(() => {
  if (progress.total === 0) return 0
  return Math.round((progress.current / progress.total) * 100)
})

const modelOptions = computed(() => {
  const p = llmStore.activeProfile
  if (!p) return []
  return p.models.map(m => ({ label: m === p.default_model ? `★ ${m}` : m, value: m }))
})

async function generate() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  const ids = selectAll.value ? [] : selectedPaperIDs.value
  generating.value = true
  progress.current = 0
  progress.total = 0
  progress.label = ''
  markdown.value = ''
  try {
    await GenerateReviewDraft(pid, selectedModel.value ?? '', ids, customPrompt.value.trim())
  } catch (e: any) {
    generating.value = false
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(localizeBackendError(e, t))
  }
}

async function cancel() {
  try { await CancelOperation() } catch { /* ignore */ }
  generating.value = false
  message.info(t('reviewDraft.cancelled'))
}

async function save() {
  if (!markdown.value) return
  try {
    await SaveExportFile('review_draft.md', markdown.value)
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
  }
}

function onReviewProgress(data: any) {
  progress.current = data?.current ?? 0
  progress.total = data?.total ?? 0
  progress.label = data?.label ?? ''
}

function onReviewDone(data: any) {
  generating.value = false
  if (data?.stage === 'error') {
    message.error(localizeBackendError({ message: data?.error ?? '' }, t))
    return
  }
  markdown.value = data?.markdown ?? ''
}

onMounted(() => {
  EventsOn('review:progress', onReviewProgress)
  EventsOn('review:done', onReviewDone)
  if (llmStore.activeProfile) {
    selectedModel.value = llmStore.activeProfile.default_model || null
  }
  if (profileStore.activeProfileId) {
    GetDownloadedPapers(profileStore.activeProfileId).then((papers: db.Paper[]) => {
      downloadedPapers.value = papers || []
    }).catch(() => {})
  }
})

onUnmounted(() => {
  // NOTE: do NOT call EventsOff here — Wails removes ALL callbacks for
  // the event, which would kill the global progress store listener.
})
</script>

<style scoped>
.review-content {
  font-size: 14px;
  line-height: 1.6;
}
.review-content :deep(h2) { font-size: 20px; margin: 16px 0 8px; }
.review-content :deep(h3) { font-size: 17px; margin: 14px 0 6px; }
.review-content :deep(h4) { font-size: 15px; margin: 10px 0 4px; }
.review-content :deep(ul) { margin: 6px 0; padding-left: 22px; }
.review-content :deep(li) { margin: 3px 0; }
.review-content :deep(p) { margin: 8px 0; }
.review-content :deep(strong) { font-weight: 600; }
.review-content :deep(blockquote) { border-left: 3px solid var(--color-primary-500, #7c5cff); padding-left: 12px; color: var(--text-tertiary, #94a3b8); margin: 8px 0; }
</style>
