<template>
  <div class="welcome-card">
    <div class="welcome-inner">
      <div class="welcome-icon">
        <svg width="64" height="64" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect x="8" y="4" width="36" height="48" rx="4" stroke="currentColor" stroke-width="2" opacity="0.3" />
          <rect x="20" y="12" width="36" height="48" rx="4" fill="var(--color-primary-900, #2e2170)" stroke="var(--color-primary-400, #9880ff)" stroke-width="2" />
          <line x1="28" y1="24" x2="48" y2="24" stroke="var(--color-primary-400, #9880ff)" stroke-width="2" stroke-linecap="round" opacity="0.6" />
          <line x1="28" y1="32" x2="44" y2="32" stroke="var(--color-primary-400, #9880ff)" stroke-width="2" stroke-linecap="round" opacity="0.4" />
          <line x1="28" y1="40" x2="46" y2="40" stroke="var(--color-primary-400, #9880ff)" stroke-width="2" stroke-linecap="round" opacity="0.3" />
        </svg>
      </div>
      <h2 class="welcome-title">{{ t('v2.welcome.title') }}</h2>
      <p class="welcome-description">{{ t('v2.welcome.description') }}</p>
      <n-button type="primary" size="large" @click="$emit('create')">
        {{ t('v2.welcome.createFirst') }}
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { NButton } from 'naive-ui'

const { t } = useI18n()

defineEmits<{
  create: []
}>()
</script>

<style scoped>
.welcome-card {
  display: flex;
  align-items: center;
  justify-content: center;
  height: calc(100vh - 140px);
}

.welcome-inner {
  text-align: center;
  max-width: 420px;
}

.welcome-icon {
  color: var(--color-primary-400, #9880ff);
  margin-bottom: var(--space-6, 24px);
}

.welcome-title {
  font-size: var(--text-h1, 28px);
  font-weight: var(--weight-semibold, 600);
  line-height: var(--leading-tight, 1.25);
  letter-spacing: -0.02em;
  margin: 0 0 var(--space-3, 12px) 0;
  color: var(--text-primary, #f8fafc);
}

.welcome-description {
  font-size: var(--text-body, 15px);
  line-height: var(--leading-normal, 1.6);
  color: var(--text-tertiary, #94a3b8);
  margin: 0 0 var(--space-8, 32px) 0;
}
</style>
