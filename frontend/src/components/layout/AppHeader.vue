<template>
  <div class="app-header">
    <!-- Left: logo -->
    <div class="app-header__left">
      <svg class="app-header__icon" width="24" height="24" viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
        <g stroke="url(#logo-grad)" stroke-linecap="round" stroke-linejoin="round">
          <path d="M10 6 H5 V26 H10" stroke-width="2.2" />
          <path d="M22 6 H27 V26 H22" stroke-width="2.2" />
          <path d="M14.5 26 V6 A4 4 0 0 1 14.5 14" stroke-width="2.8" />
        </g>
        <defs>
          <linearGradient id="logo-grad" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">
            <stop stop-color="#7c5cff" />
            <stop offset="1" stop-color="#46e0c0" />
          </linearGradient>
        </defs>
      </svg>
      <span class="app-header__logo">papeer</span>
    </div>

    <!-- Center: profile switcher -->
    <div class="app-header__center">
      <n-popover trigger="click" placement="bottom" :width="280" v-model:show="dropdownVisible">
        <template #trigger>
          <n-button quaternary style="font-size: 14px; font-weight: 500" data-onboarding="profile-switcher">
            <template #icon>
              <n-icon :component="SwapHorizontalOutline" />
            </template>
            {{ profileStore.activeProfile?.name || t('v2.header.noProfiles') }}
          </n-button>
        </template>

        <!-- Profile list -->
        <n-space vertical :size="0">
          <!-- Search (only if > 5 profiles) -->
          <n-input
            v-if="profileStore.profiles.length > 5"
            v-model:value="searchQuery"
            :placeholder="t('common.search')"
            size="small"
            clearable
            style="margin-bottom: 8px"
          />

          <div
            v-for="p in filteredProfiles"
            :key="p.id"
            class="profile-item"
            :class="{ active: p.id === profileStore.activeProfileId }"
            @click="selectProfile(p.id)"
          >
            <n-space align="center" :size="8" :wrap="false">
              <n-icon v-if="p.id === profileStore.activeProfileId" :component="CheckmarkOutline" size="16" />
              <span v-else style="width: 16px; display: inline-block" />
              <n-ellipsis style="max-width: 200px">{{ p.name }}</n-ellipsis>
            </n-space>
          </div>

          <n-divider style="margin: 8px 0" />

          <div class="profile-item" @click="createNew">
            <n-text type="primary" style="font-size: 13px">
              {{ t('v2.header.createProfile') }}
            </n-text>
          </div>
          <div class="profile-item" @click="manageProfiles">
            <n-text depth="3" style="font-size: 13px">
              {{ t('v2.header.manageProfiles') }}
            </n-text>
          </div>
        </n-space>
      </n-popover>
    </div>

    <!-- Right: background tasks + settings gear -->
    <div class="app-header__right">
      <BackgroundTasksIndicator />
      <n-button quaternary circle size="small" @click="openSettings">
        <template #icon>
          <n-icon :component="SettingsOutline" />
        </template>
      </n-button>
    </div>

    <!-- Profile form modal -->
    <ProfileFormModal v-model:show="showProfileModal" :profile="editingProfile" @saved="onProfileSaved" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NButton, NIcon, NPopover, NInput, NSpace,
  NDivider, NEllipsis, NText,
} from 'naive-ui'
import { SwapHorizontalOutline, CheckmarkOutline, SettingsOutline } from '@vicons/ionicons5'
import { useProfileStore } from '../../stores/profile'
import BackgroundTasksIndicator from './BackgroundTasksIndicator.vue'
import ProfileFormModal from '../ProfileFormModal.vue'
import { useOnboardingPhase } from '../../composables/useOnboarding'
import { db } from '../../../wailsjs/go/models'

const { t } = useI18n()
const router = useRouter()
const profileStore = useProfileStore()

const emit = defineEmits<{
  openSettings: []
}>()

function openSettings() {
  emit('openSettings')
}

const dropdownVisible = ref(false)
const searchQuery = ref('')
const showProfileModal = ref(false)
const editingProfile = ref<db.Profile | null>(null)

// Onboarding phase 1: profile switcher
useOnboardingPhase(1, {
  precondition: () => profileStore.profiles.length > 0,
  getSteps: () => [{
    element: '[data-onboarding="profile-switcher"]',
    popover: {
      title: t('onboarding.profiles.title'),
      description: t('onboarding.profiles.description'),
    },
  }],
})

const filteredProfiles = computed(() => {
  if (!searchQuery.value) return profileStore.profiles
  const q = searchQuery.value.toLowerCase()
  return profileStore.profiles.filter(p => p.name.toLowerCase().includes(q))
})

function selectProfile(id: number) {
  profileStore.activeProfileId = id
  dropdownVisible.value = false
  searchQuery.value = ''
}

function createNew() {
  dropdownVisible.value = false
  editingProfile.value = null
  showProfileModal.value = true
}

function manageProfiles() {
  dropdownVisible.value = false
  router.push({ name: 'settings' })
}

function onProfileSaved(_profile: db.Profile) {
  // profile store already updated
}
</script>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  height: 56px;
  padding: 0 var(--space-5);
  border-bottom: 1px solid var(--surface-border);
  gap: var(--space-4);
  background: var(--surface-card);
}

.app-header__left {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  flex-shrink: 0;
}

.app-header__icon {
  flex-shrink: 0;
}

.app-header__logo {
  font-size: 18px;
  font-weight: var(--weight-bold);
  letter-spacing: -0.02em;
  background: linear-gradient(135deg, var(--color-primary-400), var(--color-accent-400));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.app-header__center {
  flex: 1;
}

.app-header__right {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}

.profile-item {
  padding: var(--space-2) var(--space-2);
  border-radius: var(--radius-md);
  cursor: pointer;
  font-size: var(--text-sm);
  transition: background-color var(--transition-fast);
}

.profile-item:hover {
  background-color: rgba(124, 92, 255, 0.08);
}

.profile-item.active {
  background-color: rgba(124, 92, 255, 0.12);
}
</style>
