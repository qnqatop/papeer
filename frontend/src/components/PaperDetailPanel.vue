<template>
  <div v-if="!paper" class="detail-empty">
    <n-text depth="3" class="text-body">{{ t('v2.papers.selectPaper') }}</n-text>
  </div>

  <div v-else class="detail-panel">
    <!-- Detail content (shown when viewMode === 'details') -->
    <n-scrollbar v-if="viewMode === 'details'" style="max-height: 100%">
      <div class="detail-content">
        <!-- Close button -->
        <n-button quaternary circle size="tiny" class="detail-close" @click="$emit('close')">
          <template #icon><n-icon :component="CloseOutline" size="16" /></template>
        </n-button>

        <!-- 1. Title + authors + venue/year -->
        <section class="detail-section">
          <h3 class="detail-title">{{ paper.title }}</h3>
          <p v-if="paper.authors?.length" class="detail-authors">
            {{ paper.authors.join(', ') }}
          </p>
          <p v-if="paper.venue || paper.year" class="detail-venue">
            <template v-if="paper.venue">{{ paper.venue }}</template>
            <template v-if="paper.venue && paper.year"> · </template>
            <template v-if="paper.year">{{ paper.year }}</template>
          </p>
        </section>

        <!-- 2. Badges: score, citations, axes, status -->
        <n-space :size="6" :wrap="true" class="detail-badges">
          <n-tag :type="paper.pre_score >= 5 ? 'success' : paper.pre_score >= 3 ? 'warning' : 'default'" size="small" round>
            {{ t('papers.colScore') }}: {{ paper.pre_score }}
          </n-tag>
          <n-tag size="small" round>
            {{ t('papers.colCitations') }} {{ paper.citation_count ?? 0 }}
          </n-tag>
          <n-tag v-if="paper.axis_count > 1" size="small" type="warning" round>
            {{ t('papers.axisCount', { count: paper.axis_count }) }}
          </n-tag>
          <n-tag :type="statusColor(paper.status)" size="small" round>
            {{ statusLabel(paper.status) }}
          </n-tag>
          <n-tag v-if="paper.ai_match_score > 0" size="small" type="info" round>
            {{ t('papers.AIMatch') }}: {{ paper.ai_match_score }}
          </n-tag>
        </n-space>

        <!-- Sources -->
        <p v-if="paper.sources?.length" class="detail-sources">
          {{ t('papers.foundIn') }} {{ paper.sources.join(', ') }}
        </p>

        <!-- 3. Action buttons -->
        <div class="detail-actions">
          <n-button-group size="small">
            <n-button
              :type="paper.status === 'approved' ? 'success' : 'default'"
              @click="$emit('setStatus', paper, paper.status === 'approved' ? 'new' : 'approved')"
            >
              {{ t('papers.approve') }}
            </n-button>
            <n-button
              :type="paper.status === 'rejected' ? 'error' : 'default'"
              @click="$emit('setStatus', paper, paper.status === 'rejected' ? 'new' : 'rejected')"
            >
              {{ t('papers.reject') }}
            </n-button>
          </n-button-group>
          <n-rate
            :value="paper.user_score ?? 0"
            :count="5"
            @update:value="(v: number) => $emit('setScore', paper!, v)"
          />
        </div>

        <!-- 4. DOI / ArXiv (compact) -->
        <p v-if="paper.doi || paper.arxiv_id" class="detail-ids">
          <template v-if="paper.doi">
            DOI: {{ paper.doi }}
            <n-button text size="tiny" class="copy-btn" @click="copyId(paper.doi!)" :title="t('papers.copyDoi')">⧉</n-button>
          </template>
          <template v-if="paper.doi && paper.arxiv_id"> · </template>
          <template v-if="paper.arxiv_id">
            arXiv: {{ paper.arxiv_id }}
            <n-button text size="tiny" class="copy-btn" @click="copyId(paper.arxiv_id!)" :title="t('papers.copyArxiv')">⧉</n-button>
          </template>
        </p>

        <!-- 5. Abstract -->
        <section v-if="paper.abstract" class="detail-section">
          <h4 class="detail-section-title">{{ t('papers.abstract') }}</h4>
          <p class="detail-abstract">{{ paper.abstract }}</p>
        </section>

        <!-- 6. AI Summary -->
        <PaperSummary
          v-if="paper.status === 'downloaded'"
          :paper-i-d="paper.id"
          :has-p-d-f="true"
        />

        <!-- 7. Notes + Tags -->
        <section class="detail-section">
          <h4 class="detail-section-title">{{ t('papers.notes') }}</h4>
          <n-input
            :value="paper.notes"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :placeholder="t('papers.notesPlaceholder')"
            @blur="(e: FocusEvent) => $emit('saveNotes', paper!, (e.target as HTMLTextAreaElement).value)"
          />
        </section>

        <section v-if="tags.length > 0" class="detail-section">
          <h4 class="detail-section-title">{{ t('tags.paperTags') }}</h4>
          <n-select
            :value="paperTagIDs"
            :options="tagOptions"
            multiple
            size="small"
            :placeholder="t('tags.addTag')"
            @update:value="(ids: number[]) => $emit('saveTags', ids)"
          />
        </section>

        <!-- Score reasons -->
        <section v-if="paper.score_reasons?.length" class="detail-section">
          <h4 class="detail-section-title">{{ t('papers.scoreReasons') }}</h4>
          <n-space :size="4" :wrap="true">
            <n-tag v-for="r in paper.score_reasons" :key="r" size="small" type="info">{{ r }}</n-tag>
          </n-space>
        </section>

        <!-- 8. PDF -->
        <section v-if="paper.status === 'downloaded'" class="detail-section">
          <n-space :size="8" align="center">
            <n-button size="small" type="primary" @click="viewMode = 'pdf'">
              <template #icon><n-icon :component="DocumentOutline" size="16" /></template>
              {{ t('papers.openPdfEmbedded') }}
            </n-button>
            <n-button size="small" quaternary @click="openSystemViewer" :title="t('papers.openPdfSystem')">
              ↗
            </n-button>
            <span v-if="paper.pdf_source" class="text-caption" style="text-transform: none">{{ paper.pdf_source }}</span>
          </n-space>
        </section>
      </div>
    </n-scrollbar>

    <!-- PDF viewer (shown when viewMode === 'pdf') -->
    <PdfViewer
      v-if="viewMode === 'pdf' && paper"
      :paper-id="paper.id"
      :pdf-source="paper.pdf_source ?? undefined"
      :initial-page="pdfState.page"
      :initial-zoom="pdfState.zoom"
      @close="viewMode = 'details'"
      @state-save="(s) => pdfState = s"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NText, NSpace, NTag, NButton, NButtonGroup,
  NRate, NInput, NSelect, NScrollbar, NIcon,
  useMessage,
} from 'naive-ui'
import { CloseOutline, DocumentOutline } from '@vicons/ionicons5'
import PaperSummary from './PaperSummary.vue'
import PdfViewer from './PdfViewer.vue'
import { OpenSystemPDF } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

const { t } = useI18n()
const message = useMessage()

const props = defineProps<{
  paper: db.Paper | null
  tags: db.Tag[]
}>()

defineEmits<{
  setStatus: [paper: db.Paper, status: string]
  setScore: [paper: db.Paper, score: number]
  saveNotes: [paper: db.Paper, notes: string]
  saveTags: [tagIDs: number[]]
  close: []
}>()

const viewMode = ref<'details' | 'pdf'>('details')
const pdfState = ref({ page: 1, zoom: 0 })

async function copyId(value: string) {
  try {
    await navigator.clipboard.writeText(value)
    message.success(t('common.copied'))
  } catch {
    message.error(t('common.copyFailed'))
  }
}

// Reset view mode when paper changes
watch(() => props.paper?.id, () => {
  viewMode.value = 'details'
})

const paperTagIDs = computed(() =>
  props.paper?.tags?.map(t => t.tag_id) ?? []
)

const tagOptions = computed(() =>
  props.tags.map(t => ({ label: t.name, value: t.id }))
)

async function openSystemViewer() {
  if (!props.paper) return
  try {
    await OpenSystemPDF(props.paper.id)
  } catch (e: any) {
    message.error(t('papers.openPdfFailed'))
  }
}

function statusColor(status: string): 'success' | 'error' | 'info' | 'default' {
  switch (status) {
    case 'approved': return 'success'
    case 'rejected': return 'error'
    case 'downloaded': return 'info'
    default: return 'default'
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'new': return t('papers.statusNew')
    case 'approved': return t('papers.statusApproved')
    case 'rejected': return t('papers.statusRejected')
    case 'downloaded': return t('papers.statusDownloaded')
    default: return status
  }
}
</script>

<style scoped>
.detail-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 400px;
}

.detail-panel {
  height: 100%;
}

.detail-content {
  padding: var(--space-5, 20px);
  position: relative;
}

.detail-close {
  position: absolute;
  top: var(--space-4, 16px);
  right: var(--space-4, 16px);
}

.detail-section {
  margin-bottom: var(--space-5, 20px);
}

.detail-title {
  font-size: var(--text-h3, 18px);
  font-weight: var(--weight-semibold, 600);
  line-height: var(--leading-snug, 1.35);
  margin: 0 0 var(--space-2, 8px) 0;
  padding-right: 32px;
  color: var(--text-primary, #f8fafc);
}

.detail-authors {
  font-size: var(--text-sm, 13px);
  color: var(--text-secondary, #cbd5e1);
  margin: 0 0 var(--space-1, 4px) 0;
  line-height: var(--leading-normal, 1.6);
}

.detail-venue {
  font-size: 12px;
  color: var(--text-tertiary, #94a3b8);
  margin: 0 0 var(--space-3, 12px) 0;
}

.detail-badges {
  margin-bottom: var(--space-3, 12px);
}

.detail-sources {
  font-size: var(--text-caption, 11px);
  color: var(--text-tertiary, #94a3b8);
  margin: 0 0 var(--space-3, 12px) 0;
}

.detail-actions {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  margin-bottom: var(--space-5, 20px);
  padding-bottom: var(--space-4, 16px);
  border-bottom: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08));
}

.detail-ids {
  font-size: 12px;
  color: var(--text-tertiary, #94a3b8);
  margin: 0 0 var(--space-3, 12px) 0;
  font-family: var(--font-mono);
}

.copy-btn {
  margin-left: 4px;
  opacity: 0.6;
  cursor: pointer;
}
.copy-btn:hover {
  opacity: 1;
}

.detail-section-title {
  font-size: var(--text-sm, 13px);
  font-weight: var(--weight-semibold, 600);
  color: var(--text-secondary, #cbd5e1);
  margin: 0 0 var(--space-2, 8px) 0;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  font-size: 11px;
}

.detail-abstract {
  font-size: var(--text-sm, 13px);
  line-height: var(--leading-normal, 1.6);
  white-space: pre-wrap;
  color: var(--text-secondary, #cbd5e1);
  margin: 0;
}
</style>
