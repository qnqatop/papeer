<template>
  <!-- Per-paper download log, aggregated from the flat event stream so parallel
       workers don't interleave. One card per paper, sorted by start order.
       Shared between the DownloadModal and the header BackgroundTasksIndicator
       popover so both surfaces show identical per-source attempt detail. -->
  <n-scrollbar v-if="paperCards.length > 0" :style="{ maxHeight }" ref="scrollRef">
    <div
      v-for="card in paperCards"
      :key="card.paperID"
      class="paper-card"
      :class="{ done: card.status === 'done', fail: card.status === 'fail', active: card.status === 'pending' }"
    >
      <div class="paper-card-head">
        <span class="paper-idx">[{{ card.idx }}/{{ card.total }}]</span>
        <span class="paper-status">
          <template v-if="card.status === 'done'">✓</template>
          <template v-else-if="card.status === 'fail'">✗</template>
          <template v-else>⟳</template>
        </span>
        <span class="paper-title" :title="card.title">{{ card.title }}</span>
      </div>

      <div class="paper-card-line">
        <template v-if="card.status === 'done'">
          <n-text style="color: var(--color-success-500, #22c55e)">
            {{ t('download.downloadedVia', { source: card.successSource }) }}
          </n-text>
        </template>
        <template v-else-if="card.status === 'fail'">
          <n-text style="color: var(--color-error-500, #ef4444)">
            {{ t('download.failedShort') }}
          </n-text>
        </template>
        <template v-else>
          <n-text depth="3">
            {{ card.currentAction }}
          </n-text>
        </template>
      </div>

      <div v-if="card.attempts.length > 0" class="paper-card-attempts">
        <span v-for="(att, i) in card.attempts" :key="i" class="attempt-chip" :class="{ ok: att.ok }">
          <template v-if="att.ok">✓ {{ att.source }}</template>
          <template v-else>{{ att.source }}<span v-if="att.reason" class="reason"> — {{ att.reason }}</span></template>
        </span>
      </div>
    </div>
  </n-scrollbar>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NScrollbar, NText } from 'naive-ui'
import { useProgressStore } from '../stores/progress'

const { t } = useI18n()
const progressStore = useProgressStore()

withDefaults(defineProps<{ maxHeight?: string }>(), { maxHeight: '360px' })

const scrollRef = ref<InstanceType<typeof NScrollbar> | null>(null)

interface AttemptChip {
  source: string
  ok: boolean
  reason: string
}

interface PaperCard {
  paperID: number
  idx: number
  total: number
  title: string
  status: 'pending' | 'done' | 'fail'
  currentAction: string
  attempts: AttemptChip[]
  successSource: string
}

// Aggregate the flat event stream into one card per paper. Each card tracks
// the running list of source attempts so the user can see exactly which
// resolver took the PDF (or why all of them gave up). Sorted by start index
// so the order stays stable even when workers race.
const paperCards = computed<PaperCard[]>(() => {
  const byID = new Map<number, PaperCard>()

  for (const ev of progressStore.downloadEvents) {
    if (ev.type === 'complete') continue
    if (!ev.paper_id) continue

    let card = byID.get(ev.paper_id)
    if (!card) {
      card = {
        paperID: ev.paper_id,
        idx: ev.current || 0,
        total: ev.total || 0,
        title: ev.title || '',
        status: 'pending',
        currentAction: t('download.queued'),
        attempts: [],
        successSource: '',
      }
      byID.set(ev.paper_id, card)
    }

    if (ev.title && !card.title) card.title = ev.title
    if (ev.current) card.idx = ev.current
    if (ev.total) card.total = ev.total

    switch (ev.type) {
      case 'start':
        card.currentAction = t('download.queued')
        break
      case 'resolving':
        card.currentAction = t('download.trying', { source: ev.source })
        // Reserve a chip we'll fill in once we know success/failure for it.
        card.attempts.push({ source: ev.source, ok: false, reason: '' })
        break
      case 'downloading':
        card.currentAction = t('download.downloadingVia', { source: ev.source })
        break
      case 'done':
        card.status = 'done'
        card.successSource = ev.source
        markLastAttempt(card, ev.source, true, '')
        break
      case 'fail':
        card.status = 'fail'
        card.currentAction = t('download.failedShort')
        // ev.error has the aggregated per-source reasons. Replace attempts
        // with the parsed list so we can show clean chips instead of one
        // giant string.
        if (ev.error) {
          card.attempts = parseFailReasons(ev.error)
        }
        break
    }
  }

  return Array.from(byID.values()).sort((a, b) => a.idx - b.idx)
})

function markLastAttempt(card: PaperCard, source: string, ok: boolean, reason: string) {
  for (let i = card.attempts.length - 1; i >= 0; i--) {
    if (card.attempts[i].source === source) {
      card.attempts[i].ok = ok
      card.attempts[i].reason = reason
      return
    }
  }
  card.attempts.push({ source, ok, reason })
}

// parseFailReasons turns the engine's aggregated error string
//   "all sources exhausted: arxiv: 429; openalex: no OA location; ..."
// into structured chips.
function parseFailReasons(err: string): AttemptChip[] {
  const stripped = err.replace(/^all sources exhausted:\s*/i, '')
  if (!stripped) return []
  return stripped.split(';').map(part => {
    const seg = part.trim()
    const colon = seg.indexOf(':')
    if (colon < 0) return { source: seg, ok: false, reason: '' }
    return {
      source: seg.slice(0, colon).trim(),
      ok: false,
      reason: seg.slice(colon + 1).trim(),
    }
  }).filter(c => c.source)
}

watch(() => paperCards.value.length, async () => {
  await nextTick()
  scrollRef.value?.scrollTo({ top: 999999 })
})
</script>

<style scoped>
.paper-card {
  padding: 8px 10px;
  margin-bottom: 6px;
  border-radius: 6px;
  border-left: 3px solid transparent;
  background: rgba(255, 255, 255, 0.03);
  font-size: 12px;
  line-height: 1.45;
}
.paper-card.done    { border-left-color: #22c55e; }
.paper-card.fail    { border-left-color: #ef4444; }
.paper-card.active  { border-left-color: #f59e0b; }

.paper-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}
.paper-idx {
  font-family: monospace;
  color: var(--text-tertiary, #94a3b8);
  font-size: 11px;
  flex-shrink: 0;
}
.paper-status {
  width: 14px;
  text-align: center;
  font-weight: 700;
  flex-shrink: 0;
}
.paper-card.done .paper-status { color: #22c55e; }
.paper-card.fail .paper-status { color: #ef4444; }
.paper-card.active .paper-status { color: #f59e0b; }

.paper-title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
}

.paper-card-line {
  margin-left: 22px;
  font-size: 12px;
}

.paper-card-attempts {
  margin-left: 22px;
  margin-top: 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.attempt-chip {
  font-family: monospace;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  background: rgba(239, 68, 68, 0.08);
  color: rgba(239, 68, 68, 0.85);
  white-space: nowrap;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.attempt-chip.ok {
  background: rgba(34, 197, 94, 0.12);
  color: rgba(34, 197, 94, 0.95);
}
.attempt-chip .reason {
  opacity: 0.7;
}
</style>
