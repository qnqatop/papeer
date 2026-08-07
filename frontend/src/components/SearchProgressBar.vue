<template>
  <Transition name="slide-up">
    <div v-if="progressStore.searching" class="search-progress-bar">
      <n-space align="center" justify="space-between" :wrap="false">
        <n-space align="center" :size="12" :wrap="false" style="flex: 1; min-width: 0">
          <n-spin :size="14" />
          <n-progress
            type="line"
            :percentage="percent"
            :height="6"
            :show-indicator="false"
            :processing="true"
            style="width: 200px; flex-shrink: 0"
          />
          <n-text style="font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis">
            {{ progressStore.lastEvent }}
          </n-text>
        </n-space>
        <n-space :size="8" :wrap="false" style="flex-shrink: 0">
          <n-button size="small" quaternary @click="$emit('openLog')">
            {{ t('v2.searchLog.expandLog') }}
          </n-button>
          <n-button size="small" type="error" quaternary @click="$emit('cancel')">
            {{ t('v2.searchLog.cancelSearch') }}
          </n-button>
        </n-space>
      </n-space>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSpace, NButton, NProgress, NSpin, NText } from 'naive-ui'
import { useProgressStore } from '../stores/progress'

const { t } = useI18n()
const progressStore = useProgressStore()

defineEmits<{
  openLog: []
  cancel: []
}>()

const percent = computed(() => {
  if (progressStore.total === 0) return 0
  return Math.round((progressStore.current / progressStore.total) * 100)
})
</script>

<style scoped>
.search-progress-bar {
  position: fixed;
  bottom: 0;
  left: 200px;
  right: 0;
  padding: 8px 16px;
  background: rgba(15, 23, 42, 0.95);
  border-top: 1px solid var(--surface-border, rgba(148, 163, 184, 0.12));
  z-index: 100;
  backdrop-filter: blur(8px);
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: transform 0.25s ease, opacity 0.25s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
