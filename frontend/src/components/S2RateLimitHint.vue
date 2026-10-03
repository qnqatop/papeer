<template>
  <!-- Shown next to failed downloads whose S2 lookups were rate-limited:
       an S2 API key (Settings → Search) lifts the shared anonymous limit. -->
  <div class="s2-hint">
    <n-text depth="3" class="s2-hint-text">{{ t('download.s2RateLimitHint') }}</n-text>
    <n-button text size="tiny" type="primary" class="s2-hint-link" @click="openSearchSettings">
      {{ t('download.openSearchSettings') }} →
    </n-button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NButton, NText } from 'naive-ui'
import { SEARCH_SETTINGS_ROUTE } from '../utils/errors'

const emit = defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const router = useRouter()

function openSearchSettings() {
  emit('navigate')
  router.push(SEARCH_SETTINGS_ROUTE)
}
</script>

<style scoped>
.s2-hint {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 2px 6px;
  font-size: 11px;
  line-height: 1.4;
  padding: 4px 6px;
  margin-top: 4px;
  border-radius: 4px;
  background: var(--surface-hover);
}
.s2-hint-text {
  font-size: 11px;
}
.s2-hint-link {
  font-size: 11px;
}
</style>
