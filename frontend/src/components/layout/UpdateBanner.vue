<template>
  <n-alert
    v-if="visible"
    type="info"
    :bordered="false"
    closable
    class="update-banner"
    @close="dismiss"
  >
    <div class="update-banner__row">
      <span>{{ t('updates.banner.available', { version }) }}</span>
      <n-button size="small" type="primary" @click="goUpdate">
        {{ t('updates.banner.action') }}
      </n-button>
    </div>
  </n-alert>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton } from 'naive-ui'
import { PendingUpdate, SaveSetting } from '../../../wailsjs/go/app/App'
import { EventsOn } from '../../../wailsjs/runtime/runtime'

const { t } = useI18n()
const router = useRouter()

const visible = ref(false)
const version = ref('')

let unlisten: (() => void) | null = null

function show(info: { latestVersion?: string } | null) {
  if (!info || !info.latestVersion) return
  version.value = info.latestVersion
  visible.value = true
}

onMounted(async () => {
  unlisten = EventsOn('update:available', (info: any) => show(info))
  // Backfill in case the startup event fired before the listener was bound.
  try {
    const pending = await PendingUpdate()
    if (pending) show(pending)
  } catch {
    /* no pending update */
  }
})

onUnmounted(() => unlisten?.())

function goUpdate() {
  visible.value = false
  router.push({ name: 'settings', hash: '#updates' })
}

function dismiss() {
  visible.value = false
  // Remember the dismissed version so the banner doesn't reappear after restart.
  SaveSetting('update_dismissed_version', version.value).catch(() => {})
}
</script>

<style scoped>
.update-banner {
  border-radius: 0;
}
.update-banner__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
