<template>
  <n-layout style="height: 100vh">
    <AppHeader @open-settings="router.push({ name: 'settings' })" />
    <UpdateBanner />
    <n-layout has-sider style="height: calc(100vh - 56px)">
      <n-layout-sider
        bordered
        :width="210"
        :collapsed-width="64"
        show-trigger
        collapse-mode="width"
      >
        <n-menu
          :value="currentRoute"
          :options="menuOptions"
          @update:value="handleMenuClick"
          style="margin-top: 12px; padding: 0 8px"
        />
      </n-layout-sider>
      <n-layout-content style="padding: 32px">
        <!-- Welcome card when no profiles exist (except on settings page) -->
        <WelcomeCard v-if="profileStore.profiles.length === 0 && !profileStore.loading && route.name !== 'settings'" @create="showWelcomeProfileModal = true" />
        <router-view v-else />
      </n-layout-content>
    </n-layout>
    <ProfileFormModal v-model:show="showWelcomeProfileModal" />
    <ProfileFormModal
      v-model:show="showDemoProfileModal"
      :initial="demoProfileInitial"
      @saved="onDemoProfileSaved"
    />
    <ProfileFormModal
      v-model:show="showNeedsEmailModal"
      :profile="needsEmailProfile"
      @saved="onNeedsEmailSaved"
    />
  </n-layout>

  <!-- Welcome modal (phase 0) -->
  <n-modal v-model:show="showWelcome" :mask-closable="false" preset="card" :title="t('onboarding.welcome.title')" style="width: 480px">
    <n-text style="white-space: pre-line; line-height: 1.7; font-size: 15px">
      {{ t('onboarding.welcome.description') }}
    </n-text>
    <template #action>
      <n-space justify="end">
        <n-button @click="skipOnboarding">{{ t('onboarding.skipAll') }}</n-button>
        <n-button type="primary" @click="startOnboarding">{{ t('onboarding.startTour') }}</n-button>
      </n-space>
    </template>
  </n-modal>

  <!-- Complete modal (phase 6) -->
  <n-modal v-model:show="showComplete" preset="card" :title="t('onboarding.complete.title')" style="width: 480px">
    <n-text style="white-space: pre-line; line-height: 1.7; font-size: 15px">
      {{ t('onboarding.complete.description') }}
    </n-text>
    <template #action>
      <n-space justify="end">
        <n-button v-if="onboardingStore.isDemoMode" @click="deleteDemoAndClose">
          {{ t('onboarding.deleteDemo') }}
        </n-button>
        <n-button type="primary" @click="showComplete = false">
          {{ onboardingStore.isDemoMode ? t('onboarding.keepDemo') : t('onboarding.done') }}
        </n-button>
      </n-space>
    </template>
  </n-modal>

  <!-- Skip confirm: offer to delete demo data -->
  <n-modal v-model:show="showSkipConfirm" preset="dialog" type="warning" :title="t('onboarding.skipConfirmTitle')" :positive-text="t('onboarding.deleteDemo')" :negative-text="t('onboarding.keepDemo')" @positive-click="deleteDemoAndSkip" @negative-click="keepDemoAndSkip">
    <n-text>{{ t('onboarding.skipConfirmMessage') }}</n-text>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NLayout, NLayoutSider, NLayoutContent,
  NMenu, NIcon, NText, NSpace,
  NButton, NModal, NBadge,
  useMessage,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
  SearchOutline,
  DocumentTextOutline,
  StatsChartOutline,
} from '@vicons/ionicons5'
import { useProfileStore } from './stores/profile'
import { useProgressStore } from './stores/progress'
import { usePapersStore } from './stores/papers'
import { useOnboardingStore } from './stores/onboarding'
import { DeleteProfile, ProfilesNeedingEmail } from '../wailsjs/go/app/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { db } from '../wailsjs/go/models'
import AppHeader from './components/layout/AppHeader.vue'
import UpdateBanner from './components/layout/UpdateBanner.vue'
import WelcomeCard from './components/layout/WelcomeCard.vue'
import ProfileFormModal from './components/ProfileFormModal.vue'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const profileStore = useProfileStore()
const progressStore = useProgressStore()
const papersStore = usePapersStore()
const onboardingStore = useOnboardingStore()
const message = useMessage()

const showWelcomeProfileModal = ref(false)
const showDemoProfileModal = ref(false)
const showNeedsEmailModal = ref(false)
const needsEmailProfile = ref<db.Profile | null>(null)
const demoProfileInitial = {
  name: 'Demo Research',
  email: '',
  pdf_dir: '',
  year_min: 2020,
  max_per_query: 10,
}

const currentRoute = computed(() => route.name as string)
const showWelcome = ref(false)
const showComplete = ref(false)
const showSkipConfirm = ref(false)

const radarNewCount = ref(0)

// Redirect to home whenever we end up with zero profiles, so the user always
// sees the WelcomeCard. Settings is always accessible.
watch(
  [() => profileStore.profiles.length, () => profileStore.loading],
  ([len, loading]) => {
    if (len === 0 && !loading && route.name !== 'settings') {
      router.replace({ name: 'search' })
    }
  },
  { immediate: false },
)

onMounted(async () => {
  await profileStore.fetchProfiles()

  progressStore.setNotify((type, content) => {
    message[type](content, { duration: 5000 })
  })

  progressStore.setOnSearchDone(() => {
    if (papersStore.filter.profile_id) {
      papersStore.fetchPapers()
    }
  })

  progressStore.setOnDownloadDone(() => {
    if (papersStore.filter.profile_id) {
      papersStore.fetchPapers()
    }
  })

  // Radar: listen for new paper notifications.
  EventsOn('radar:done', (data: any) => {
    const count = data?.new_papers || 0
    if (count > 0) {
      radarNewCount.value += count
      message.success(t('monitoring.foundNew', { count }))
    }
  })

  // If any existing profile lacks a valid email, force the user to fix it.
  EventsOn('profile:needs_email', (p: db.Profile) => {
    if (!needsEmailProfile.value && !showDemoProfileModal.value && !showWelcomeProfileModal.value) {
      needsEmailProfile.value = p
      showNeedsEmailModal.value = true
    }
  })

  progressStore.startListening()

  // Backfill: in case the startup event fired before our listener bound, ask
  // the backend now for any profiles that still need a valid email.
  try {
    const bad = await ProfilesNeedingEmail()
    if (bad && bad.length > 0 && !needsEmailProfile.value) {
      needsEmailProfile.value = bad[0]
      showNeedsEmailModal.value = true
    }
  } catch {
    // Non-fatal: backend may not be ready yet.
  }

  // Show welcome modal on first launch
  if (onboardingStore.isFirstLaunch) {
    showWelcome.value = true
  }
})

onUnmounted(() => {
  EventsOff('radar:done', 'profile:needs_email')
  progressStore.stopListening()
})

// Watch for tour restart from Settings
watch(() => onboardingStore.pendingRestart, (val) => {
  if (val) {
    onboardingStore.pendingRestart = false
    showWelcome.value = true
    router.push({ name: 'search' })
  }
})

// Watch for phase 5 completion → show complete modal (phase 6)
watch(
  () => [...onboardingStore.completedPhases],
  (phases) => {
    if (phases.includes(5) && !phases.includes(6)) {
      showComplete.value = true
      onboardingStore.markPhaseComplete(6)
    }
  },
)

function startOnboarding() {
  showWelcome.value = false
  showDemoProfileModal.value = true
}

async function onDemoProfileSaved(profile: db.Profile) {
  showDemoProfileModal.value = false
  onboardingStore.setDemoProfileId(profile.id)
  profileStore.activeProfileId = profile.id
  await profileStore.fetchProfiles()

  onboardingStore.markPhaseComplete(0)
  onboardingStore.markPhaseComplete(1)
  onboardingStore.persist()

  router.push({ name: 'search' })
}

function onNeedsEmailSaved(_profile: db.Profile) {
  showNeedsEmailModal.value = false
  needsEmailProfile.value = null
  message.success(t('profiles.emailSaved'))
}

function skipOnboarding() {
  showWelcome.value = false
  onboardingStore.skipAll()
}

watch(() => onboardingStore.skippedAll, (skipped) => {
  if (skipped && onboardingStore.isDemoMode) {
    showSkipConfirm.value = true
  }
})

async function deleteDemoProfile() {
  const demoId = onboardingStore.demoProfileId
  if (!demoId) return
  try {
    await DeleteProfile(demoId)
    onboardingStore.setDemoProfileId(null)
    if (profileStore.activeProfileId === demoId) {
      profileStore.activeProfileId = null
    }
    await profileStore.fetchProfiles()
  } catch {
    onboardingStore.setDemoProfileId(null)
  }
}

async function deleteDemoAndClose() {
  await deleteDemoProfile()
  showComplete.value = false
  message.success(t('onboarding.demoDeleted'))
}

async function deleteDemoAndSkip() {
  await deleteDemoProfile()
  message.success(t('onboarding.demoDeleted'))
}

function keepDemoAndSkip() {
  // just close — demo stays
}

function renderIcon(icon: any) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions = computed<MenuOption[]>(() => {
  // Hide all menu items when no profiles exist — user must create one first.
  if (profileStore.profiles.length === 0) return []

  return [
    { label: t('nav.search'), key: 'search', icon: renderIcon(SearchOutline) },
    { label: () => h('span', { style: 'display:flex;align-items:center;gap:6px' }, [
      t('nav.papers'),
      radarNewCount.value > 0 ? h(NBadge, { value: radarNewCount.value, type: 'error' }) : null,
    ]), key: 'papers', icon: renderIcon(DocumentTextOutline) },
    { label: t('nav.analysis'), key: 'analysis', icon: renderIcon(StatsChartOutline) },
  ]
})

function handleMenuClick(key: string) {
  if (key === 'papers') {
    radarNewCount.value = 0
  }
  router.push({ name: key })
}
</script>
