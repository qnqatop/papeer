<template>
  <div>
    <n-space align="center" justify="space-between" style="margin-bottom: 16px">
      <n-text depth="3" style="font-size: 13px">{{ t('recs.subtitle') }}</n-text>
      <n-select
        v-model:value="selectedAxisId"
        :options="axisOptions"
        style="width: 220px"
        size="small"
      />
    </n-space>

    <n-spin :show="loading">
      <n-empty
        v-if="!loading && papers.length === 0"
        :description="t('recs.noRecs')"
        style="margin-top: 60px"
      >
        <template #extra>
          <n-text depth="3">{{ t('recs.noRecsHint') }}</n-text>
        </template>
      </n-empty>

      <n-grid v-else :cols="1" :x-gap="0" :y-gap="12">
        <n-gi v-for="paper in papers" :key="paper.id">
          <n-card
            size="small"
            hoverable
            style="border-left: 3px solid; cursor: pointer"
            :style="{ borderLeftColor: matchColor(paper.ai_match_score) }"
            @click="emit('openDetail', paper)"
          >
            <template #header>
              <n-space align="center" :size="8">
                <n-tag :type="matchType(paper.ai_match_score)" size="small" round>
                  {{ matchLabel(paper.ai_match_score) }}
                </n-tag>
                <n-text style="font-size: 14px; font-weight: 500">
                  {{ paper.title }}
                </n-text>
              </n-space>
            </template>
            <template #header-extra>
              <n-space :size="4">
                <n-button size="tiny" type="success" secondary @click.stop="approve(paper)">
                  {{ t('recs.approve') }}
                </n-button>
                <n-button size="tiny" type="error" secondary @click.stop="reject(paper)">
                  {{ t('recs.reject') }}
                </n-button>
              </n-space>
            </template>

            <n-space vertical :size="6">
              <n-text depth="3" style="font-size: 12px">
                <template v-if="paper.authors && paper.authors.length">
                  {{ paper.authors.slice(0, 3).join(', ') }}{{ paper.authors.length > 3 ? ' et al.' : '' }}
                </template>
                <template v-if="paper.year"> · {{ paper.year }}</template>
                <template v-if="paper.citation_count"> · {{ paper.citation_count }} {{ t('recs.citations') }}</template>
                <template v-if="paper.venue"> · {{ paper.venue }}</template>
              </n-text>
              <n-text
                v-if="paper.abstract"
                depth="2"
                style="font-size: 13px; line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden"
              >
                {{ paper.abstract }}
              </n-text>
            </n-space>
          </n-card>
        </n-gi>
      </n-grid>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NText, NSpace, NSelect, NSpin, NEmpty,
  NGrid, NGi, NCard, NTag, NButton,
} from 'naive-ui'
import { ListPapers, ListAxes, UpdatePaperStatus } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useFirstVisitHint } from '../composables/useFirstVisitHint'

const props = defineProps<{ profileId: number }>()
const emit = defineEmits<{
  (e: 'statusChanged'): void
  (e: 'openDetail', paper: db.Paper): void
}>()

const { t } = useI18n()

useFirstVisitHint('recs', {
  steps: () => [{
    popover: { title: t('hints.recs.title'), description: t('hints.recs.description') },
  }],
})

const axes = ref<db.Axis[]>([])
const selectedAxisId = ref<number | null>(null)
const papers = ref<db.Paper[]>([])
const loading = ref(false)

const axisOptions = computed(() => {
  const opts = axes.value.map(a => ({
    label: a.axis_key + (a.description ? ` — ${a.description}` : ''),
    value: a.id,
  }))
  if (opts.length > 1) {
    opts.unshift({ label: t('recs.allAxes'), value: 0 as any })
  }
  return opts
})

async function loadAxes() {
  if (!props.profileId) return
  axes.value = await ListAxes(props.profileId)
  if (axes.value.length > 0 && selectedAxisId.value == null) {
    selectedAxisId.value = axes.value.length > 1 ? 0 : axes.value[0].id
  }
}

async function loadRecs() {
  if (!props.profileId) return
  loading.value = true
  try {
    if (selectedAxisId.value && selectedAxisId.value !== 0) {
      const result = await ListPapers(new db.PaperFilter({
        profile_id: props.profileId,
        axis_id: selectedAxisId.value,
        status: 'new',
        sort_by: 'ai_match_score',
        limit: 10,
        offset: 0,
      }))
      papers.value = (result.papers || []).filter(p => p.ai_match_score > 0)
    } else {
      const allPapers: db.Paper[] = []
      for (const axis of axes.value) {
        const result = await ListPapers(new db.PaperFilter({
          profile_id: props.profileId,
          axis_id: axis.id,
          status: 'new',
          sort_by: 'ai_match_score',
          limit: 10,
          offset: 0,
        }))
        const scored = (result.papers || []).filter(p => p.ai_match_score > 0)
        allPapers.push(...scored)
      }
      const seen = new Map<number, db.Paper>()
      for (const p of allPapers) {
        const existing = seen.get(p.id)
        if (!existing || p.ai_match_score > existing.ai_match_score) {
          seen.set(p.id, p)
        }
      }
      papers.value = Array.from(seen.values())
        .sort((a, b) => b.ai_match_score - a.ai_match_score)
        .slice(0, 15)
    }
  } finally {
    loading.value = false
  }
}

function matchLabel(score: number): string {
  if (score >= 75) return t('recs.strongMatch')
  if (score >= 50) return t('recs.goodMatch')
  if (score >= 30) return t('recs.fairMatch')
  return t('recs.weakMatch')
}

function matchType(score: number): 'success' | 'info' | 'warning' | 'default' {
  if (score >= 75) return 'success'
  if (score >= 50) return 'info'
  if (score >= 30) return 'warning'
  return 'default'
}

function matchColor(score: number): string {
  if (score >= 75) return '#22c55e'
  if (score >= 50) return '#7c5cff'
  if (score >= 30) return '#f59e0b'
  return '#64748b'
}

async function approve(paper: db.Paper) {
  await UpdatePaperStatus(paper.id, 'approved')
  papers.value = papers.value.filter(p => p.id !== paper.id)
  emit('statusChanged')
}

async function reject(paper: db.Paper) {
  await UpdatePaperStatus(paper.id, 'rejected')
  papers.value = papers.value.filter(p => p.id !== paper.id)
  emit('statusChanged')
}

watch(() => props.profileId, async () => {
  await loadAxes()
  await loadRecs()
})

watch(selectedAxisId, () => {
  loadRecs()
})

onMounted(async () => {
  await loadAxes()
  await loadRecs()
  EventsOn('ai_scores_updated', () => loadRecs())
})

onUnmounted(() => {
  EventsOff('ai_scores_updated')
})
</script>
