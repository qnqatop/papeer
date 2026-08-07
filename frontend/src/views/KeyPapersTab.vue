<template>
  <n-space vertical :size="16">
    <n-card v-if="!profileStore.activeProfileId" :title="t('keyPapers.title')">
      <n-empty :description="t('common.selectProfileFirst')" />
    </n-card>

    <template v-else>
      <n-card v-if="loading">
        <div style="display: flex; justify-content: center; padding: 48px 0">
          <n-spin size="large" />
        </div>
      </n-card>

      <n-card v-else-if="isEmpty" :title="t('keyPapers.title')">
        <n-empty :description="t('keyPapers.empty')" />
      </n-card>

      <n-grid v-else :cols="3" :x-gap="16" :y-gap="16">
        <n-gi>
          <n-card :title="t('keyPapers.mostCited')" size="small">
            <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">{{ t('keyPapers.mostCitedHint') }}</n-text>
            <n-empty v-if="mostCited.length === 0" :description="t('keyPapers.sectionEmpty')" size="small" />
            <KeyPaperRow v-for="p in mostCited" :key="p.id" :paper="p" :score-label="String(p.score)" @click="openPaper(p.id)" />
          </n-card>
        </n-gi>
        <n-gi>
          <n-card :title="t('keyPapers.rising')" size="small">
            <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">{{ t('keyPapers.risingHint') }}</n-text>
            <n-empty v-if="rising.length === 0" :description="t('keyPapers.sectionEmpty')" size="small" />
            <KeyPaperRow v-for="p in rising" :key="p.id" :paper="p" :score-label="p.score.toFixed(1)" @click="openPaper(p.id)" />
          </n-card>
        </n-gi>
        <n-gi>
          <n-card :title="t('keyPapers.bridge')" size="small">
            <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">{{ t('keyPapers.bridgeHint') }}</n-text>
            <n-empty v-if="bridge.length === 0" :description="t('keyPapers.bridgeEmpty')" size="small" />
            <KeyPaperRow v-for="p in bridge" :key="p.id" :paper="p" :score-label="p.score.toFixed(3)" @click="openPaper(p.id)" />
          </n-card>
        </n-gi>
      </n-grid>
    </template>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, h, defineComponent, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NCard, NEmpty, NSpace, NSpin, NText, NTag, NGrid, NGi, useMessage } from 'naive-ui'
import { GetKeyPapers } from '../../wailsjs/go/app/App'
import { app } from '../../wailsjs/go/models'
import { useProfileStore } from '../stores/profile'
import { localizeBackendError } from '../utils/errors'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const profileStore = useProfileStore()

const result = ref<app.KeyPapersResult | null>(null)
const loading = ref(false)

const mostCited = computed(() => result.value?.most_cited ?? [])
const rising = computed(() => result.value?.rising ?? [])
const bridge = computed(() => result.value?.bridge ?? [])
const isEmpty = computed(() => mostCited.value.length === 0 && rising.value.length === 0 && bridge.value.length === 0)

function openPaper(id: number) {
  router.push({ path: '/papers', query: { paper_id: String(id) } })
}

async function loadKeyPapers() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  loading.value = true
  try {
    result.value = await GetKeyPapers(pid)
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
    result.value = null
  } finally {
    loading.value = false
  }
}

onMounted(loadKeyPapers)
watch(() => profileStore.activeProfileId, loadKeyPapers)

// Small inline row component — title + year + score, click to open in Papers.
const KeyPaperRow = defineComponent({
  props: {
    paper: { type: Object as PropType<app.KeyPaper>, required: true },
    scoreLabel: { type: String, required: true },
  },
  emits: ['click'],
  setup(props, { emit }) {
    return () => h('div', {
      class: 'key-paper-row',
      onClick: () => emit('click'),
    }, [
      h('div', { class: 'key-paper-row__title' }, props.paper.title),
      h('div', { class: 'key-paper-row__meta' }, [
        props.paper.year ? h('span', String(props.paper.year) + ' · ') : null,
        h(NTag, { size: 'tiny', round: true }, () => props.scoreLabel),
      ]),
    ])
  },
})
</script>

<style scoped>
:deep(.key-paper-row) {
  padding: 8px 0;
  border-bottom: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08));
  cursor: pointer;
}
:deep(.key-paper-row:hover) {
  background: rgba(124, 92, 255, 0.06);
}
:deep(.key-paper-row__title) {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.4;
  margin-bottom: 4px;
}
:deep(.key-paper-row__meta) {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--text-tertiary, #94a3b8);
}
</style>
