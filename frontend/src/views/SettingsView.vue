<template>
  <n-card>
    <template #header>
      <n-space align="center" :size="8">
        <n-button quaternary circle size="small" @click="router.push({ name: 'search' })">
          <template #icon>
            <n-icon :component="ArrowBackOutline" />
          </template>
        </n-button>
        {{ t('settings.title') }}
      </n-space>
    </template>
    <div class="settings-layout">
      <!-- Left: section navigation -->
      <n-menu
        v-model:value="activeSection"
        :options="sectionOptions"
        style="width: 180px; flex-shrink: 0"
      />

      <!-- Right: section content -->
      <div class="settings-content">
        <!-- Researches -->
        <template v-if="activeSection === 'researches'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.researches') }}</n-h3>
          <n-space vertical :size="12">
            <n-space justify="end">
              <n-button size="small" type="primary" @click="showProfileModal = true">
                {{ t('profiles.createProfile') }}
              </n-button>
            </n-space>
            <n-card
              v-for="p in profileStore.profiles"
              :key="p.id"
              size="small"
              :class="{ 'active-profile': p.id === profileStore.activeProfileId }"
            >
              <n-space justify="space-between" align="center">
                <n-space align="center" :size="8">
                  <n-text strong>{{ p.name }}</n-text>
                  <n-tag v-if="p.id === profileStore.activeProfileId" type="success" size="tiny">{{ t('common.active') }}</n-tag>
                </n-space>
                <n-space :size="4">
                  <n-button size="tiny" quaternary @click="profileStore.activeProfileId = p.id">{{ t('common.select') }}</n-button>
                  <n-button size="tiny" quaternary @click="editProfile(p)">{{ t('common.edit') }}</n-button>
                  <n-popconfirm @positive-click="deleteProfile(p)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error">{{ t('common.delete') }}</n-button>
                    </template>
                    {{ t('profiles.deleteConfirmMessage', { name: p.name }) }}
                  </n-popconfirm>
                </n-space>
              </n-space>
              <n-text depth="3" style="font-size: 12px; display: block; margin-top: 4px">
                {{ p.pdf_dir || t('common.notSet') }} · {{ t('profiles.yearMin') }}: {{ p.year_min }}
              </n-text>
            </n-card>
            <n-empty v-if="profileStore.profiles.length === 0" :description="t('profiles.emptyState')" />
          </n-space>
          <ProfileFormModal v-model:show="showProfileModal" :profile="editingProfile" @saved="onProfileSaved" />
        </template>

        <!-- General -->
        <template v-if="activeSection === 'general'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.general') }}</n-h3>
          <n-space vertical :size="16">
            <div>
              <n-text strong>{{ t('lang.label') }}</n-text>
              <n-select
                :value="currentLocale"
                :options="localeOptions"
                style="max-width: 200px; margin-top: 4px"
                @update:value="switchLocale"
              />
            </div>
            <div>
              <n-text strong>{{ t('settings.onboarding') }}</n-text>
              <div style="margin-top: 4px">
                <n-button @click="restartTour">
                  <template #icon><n-icon :component="HelpCircleOutline" /></template>
                  {{ t('onboarding.restartTour') }}
                </n-button>
              </div>
            </div>
          </n-space>
        </template>

        <!-- AI / LLM -->
        <template v-if="activeSection === 'aiLlm'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.aiLlm') }}</n-h3>
          <n-space vertical :size="16">
            <n-space justify="space-between" align="center">
              <n-text depth="3" style="font-size: 13px">{{ t('llm.sectionHint') }}</n-text>
              <n-button size="small" type="primary" @click="openCreateProfile">
                {{ t('llm.addProfile') }}
              </n-button>
            </n-space>

            <n-empty v-if="llmStore.profiles.length === 0" :description="t('llm.emptyState')">
              <template #extra>
                <n-button size="small" type="primary" @click="openCreateProfile">
                  {{ t('llm.addFirstProfile') }}
                </n-button>
              </template>
            </n-empty>

            <n-card
              v-for="p in llmStore.profiles"
              :key="p.id"
              size="small"
              :class="{ 'active-profile': p.is_active }"
            >
              <n-space justify="space-between" align="start" :wrap="false">
                <n-space vertical :size="6" style="min-width: 0">
                  <n-space align="center" :size="8">
                    <n-text strong>{{ p.name }}</n-text>
                    <n-tag v-if="p.is_active" type="success" size="tiny" round>● {{ t('common.active') }}</n-tag>
                  </n-space>
                  <n-text depth="3" style="font-size: 12px; font-family: var(--font-mono, monospace)">
                    {{ p.has_key ? '🔒 ' + p.key_mask : '🔑 —' }}
                  </n-text>
                  <n-space :size="4" :wrap="true">
                    <n-tag
                      v-for="m in p.models"
                      :key="m"
                      size="tiny"
                      :type="m === p.default_model ? 'primary' : 'default'"
                    >
                      {{ m === p.default_model ? '★ ' + m : m }}
                    </n-tag>
                  </n-space>
                  <n-text depth="3" style="font-size: 12px">{{ p.base_url }}</n-text>
                </n-space>
                <n-space vertical :size="4" align="end" :wrap="false">
                  <n-button size="tiny" quaternary :loading="testingId === p.id" @click="testProfile(p)">
                    {{ t('llm.test') }}
                  </n-button>
                  <n-button size="tiny" quaternary @click="editProfileLLM(p)">{{ t('common.edit') }}</n-button>
                  <n-button v-if="!p.is_active" size="tiny" quaternary type="primary" @click="setActiveProfile(p)">
                    {{ t('llm.makeActiveShort') }}
                  </n-button>
                  <n-popconfirm @positive-click="deleteLLMProfile(p)">
                    <template #trigger>
                      <n-button size="tiny" quaternary type="error">{{ t('common.delete') }}</n-button>
                    </template>
                    {{ t('llm.deleteConfirm', { name: p.name }) }}
                  </n-popconfirm>
                </n-space>
              </n-space>
            </n-card>

            <LLMProfileFormModal v-model:show="showLLMModal" :profile="editingLLMProfile" @saved="onLLMProfileSaved" />

            <n-divider style="margin: 4px 0" />

            <!-- Summary Prompt -->
            <div>
              <n-text strong>{{ t('settings.prompt.title') }}</n-text>
              <n-text depth="3" style="font-size: 13px; display: block; margin: 4px 0">{{ t('settings.prompt.hint') }}</n-text>
              <n-input
                v-model:value="summaryPrompt"
                type="textarea"
                :autosize="{ minRows: 8, maxRows: 16 }"
                :placeholder="defaultPrompt"
                style="font-family: monospace; font-size: 13px"
              />
              <n-space :size="8" style="margin-top: 8px">
                <n-button size="small" type="primary" @click="savePrompt">{{ t('settings.prompt.save') }}</n-button>
                <n-popconfirm @positive-click="resetPrompt">
                  <template #trigger>
                    <n-button size="small">{{ t('settings.prompt.reset') }}</n-button>
                  </template>
                  {{ t('settings.prompt.resetConfirm') }}
                </n-popconfirm>
              </n-space>
            </div>
          </n-space>
        </template>

        <!-- Search -->
        <template v-if="activeSection === 'search'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.search') }}</n-h3>
          <n-space vertical :size="16">
            <n-text depth="3" style="font-size: 13px">
              {{ t('v2.settings.searchDescription') }}
            </n-text>
            <n-text depth="2" style="font-size: 13px; line-height: 1.7; white-space: pre-line">
              {{ t('v2.settings.searchDetail') }}
            </n-text>
          </n-space>
        </template>

        <!-- Monitoring -->
        <template v-if="activeSection === 'monitoring'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.monitoring') }}</n-h3>
          <n-space vertical :size="12">
            <n-text depth="3" style="font-size: 13px">{{ t('monitoring.description') }}</n-text>
            <n-space :size="8" align="center">
              <n-switch :value="radarEnabled" @update:value="toggleRadar" />
              <n-text>{{ t('monitoring.enable') }}</n-text>
            </n-space>
            <n-space v-if="radarEnabled" :size="8" align="center">
              <n-text depth="3" style="min-width: 80px">{{ t('monitoring.frequency') }}</n-text>
              <n-select :value="radarFrequency" :options="radarFrequencyOptions" style="width: 200px" @update:value="saveRadarFrequency" />
            </n-space>
            <n-button :loading="runningRadar" @click="runRadarNow">
              {{ runningRadar ? t('monitoring.running') : t('monitoring.runNow') }}
            </n-button>
          </n-space>
        </template>

        <!-- Tags -->
        <template v-if="activeSection === 'tags'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.tags') }}</n-h3>
          <n-text v-if="!profileStore.activeProfile" depth="3" style="font-size: 13px">{{ t('common.selectProfileFirst') }}</n-text>
          <template v-else>
            <n-space :size="8" :wrap="true" style="margin-bottom: 12px">
              <n-tag
                v-for="tag in tags"
                :key="tag.id"
                :color="{ color: tag.color + '20', textColor: tag.color, borderColor: tag.color }"
                closable
                @close="deleteTag(tag)"
              >
                {{ tag.name }}
              </n-tag>
              <n-text v-if="tags.length === 0" depth="3" style="font-size: 13px">{{ t('tags.noTags') }}</n-text>
            </n-space>
            <n-space :size="8" align="center">
              <n-input v-model:value="newTagName" :placeholder="t('tags.namePlaceholder')" size="small" style="width: 180px" @keydown.enter="createTag" />
              <n-popover trigger="click" placement="bottom" :width="200">
                <template #trigger>
                  <div class="color-swatch-trigger" :style="{ backgroundColor: newTagColor }" :title="t('tags.color')" />
                </template>
                <n-space :size="6" :wrap="true" justify="center">
                  <div
                    v-for="color in tagSwatches"
                    :key="color"
                    class="color-swatch-option"
                    :class="{ active: newTagColor === color }"
                    :style="{ backgroundColor: color }"
                    @click="newTagColor = color"
                  />
                </n-space>
              </n-popover>
              <n-button size="small" type="primary" :disabled="!newTagName.trim()" @click="createTag">{{ t('tags.create') }}</n-button>
            </n-space>
          </template>
        </template>

        <!-- Proxy -->
        <template v-if="activeSection === 'proxy'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.proxy') }}</n-h3>
          <n-text depth="3" style="font-size: 13px; display: block; margin-bottom: 8px">{{ t('settings.proxyHint') }}</n-text>
          <n-space :size="8">
            <n-input v-model:value="proxyURL" :placeholder="t('settings.proxyPlaceholder')" style="width: 360px" @blur="saveProxy" @keydown.enter="saveProxy" />
            <n-button :loading="testingProxy" :disabled="!proxyURL" @click="testProxy">{{ t('settings.testProxy') }}</n-button>
          </n-space>

          <n-divider style="margin: 16px 0" />

          <n-text strong style="font-size: 13px; display: block; margin-bottom: 4px">{{ t('settings.s2ApiKey') }}</n-text>
          <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 8px">{{ t('settings.s2ApiKeyHint') }}</n-text>
          <n-input
            v-model:value="s2ApiKey"
            type="password"
            show-password-on="click"
            :placeholder="t('settings.s2ApiKeyPlaceholder')"
            style="width: 360px"
            @blur="saveS2ApiKey"
            @keydown.enter="saveS2ApiKey"
          />

          <n-divider style="margin: 16px 0" />

          <n-text depth="3" style="font-size: 13px; display: block; margin-bottom: 8px">{{ t('settings.checkProvidersHint') }}</n-text>
          <n-button :loading="checkingProviders" @click="checkProviders">{{ t('settings.checkProviders') }}</n-button>
          <div v-if="providerStatus.length > 0" style="margin-top: 12px; display: flex; flex-direction: column; gap: 4px">
            <div v-for="s in providerStatus" :key="s.name" style="display: flex; align-items: center; gap: 8px; font-size: 13px; font-family: var(--font-mono, monospace)">
              <span :style="{ color: s.ok ? '#10b981' : '#ef4444' }">{{ s.ok ? '✓' : '✗' }}</span>
              <span style="min-width: 120px">{{ s.name }}</span>
              <span v-if="s.ok" style="color: var(--text-tertiary)">{{ s.latency_ms }} ms</span>
              <span v-else style="color: #ef4444; font-size: 11px">{{ s.error }}</span>
            </div>
          </div>
        </template>

        <!-- About -->
        <template v-if="activeSection === 'about'">
          <n-h3 style="margin: 0 0 16px 0">{{ t('v2.settings.sections.about') }}</n-h3>
          <n-space vertical :size="8">
            <n-text style="font-size: 16px; font-weight: 700">Papeer</n-text>
            <n-text depth="2" style="font-size: 13px">{{ t('v2.settings.about.subtitle') }}</n-text>
            <n-text depth="3" style="font-size: 12px">{{ appVersion || 'dev' }}</n-text>
            <n-divider style="margin: 8px 0" />
            <n-text depth="3" style="font-size: 12px; line-height: 1.6; white-space: pre-line">
              {{ t('v2.settings.about.description') }}
            </n-text>
            <n-text depth="3" style="font-size: 11px; margin-top: 8px">
              {{ t('v2.settings.about.copyright') }}
            </n-text>
          </n-space>
        </template>
      </div>
    </div>
  </n-card>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  NCard, NSpace, NButton, NIcon, NSelect, NText, NInput,
  NTag, NSwitch, NPopconfirm, NPopover, NMenu, NH3, NEmpty,
  NDivider,
  useMessage,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { ArrowBackOutline, HelpCircleOutline } from '@vicons/ionicons5'
import { useOnboardingStore } from '../stores/onboarding'
import { useProfileStore } from '../stores/profile'
import { useProgressStore } from '../stores/progress'
import { useLLMProfilesStore } from '../stores/llmProfiles'
import { setLocale, getLocale } from '../i18n'
import { isInvalidEmailError } from '../utils/errors'
import {
  GetSettings, SaveSetting, TestProxy,
  ListTags, CreateTag as CreateTagAPI, DeleteTag as DeleteTagAPI,
  RunRadar, DeleteProfile as DeleteProfileAPI, AppVersion,
  CheckSearchProviders,
} from '../../wailsjs/go/app/App'
import { app, db } from '../../wailsjs/go/models'
import ProfileFormModal from '../components/ProfileFormModal.vue'
import LLMProfileFormModal from '../components/LLMProfileFormModal.vue'

const { t } = useI18n()
const router = useRouter()
const onboardingStore = useOnboardingStore()
const profileStore = useProfileStore()
const progressStore = useProgressStore()
const llmStore = useLLMProfilesStore()
const message = useMessage()

const activeSection = ref('researches')

const sectionOptions = computed<MenuOption[]>(() => [
  { label: t('v2.settings.sections.researches'), key: 'researches' },
  { label: t('v2.settings.sections.general'), key: 'general' },
  { label: t('v2.settings.sections.aiLlm'), key: 'aiLlm' },
  { label: t('v2.settings.sections.search'), key: 'search' },
  { label: t('v2.settings.sections.monitoring'), key: 'monitoring' },
  { label: t('v2.settings.sections.tags'), key: 'tags' },
  { label: t('v2.settings.sections.proxy'), key: 'proxy' },
  { label: t('v2.settings.sections.about'), key: 'about' },
])

// --- Profiles ---
const showProfileModal = ref(false)
const editingProfile = ref<db.Profile | undefined>(undefined)

function editProfile(p: db.Profile) {
  editingProfile.value = p
  showProfileModal.value = true
}

async function deleteProfile(p: db.Profile) {
  try {
    await DeleteProfileAPI(p.id)
    if (profileStore.activeProfileId === p.id) {
      profileStore.activeProfileId = null
    }
    await profileStore.fetchProfiles()
    message.success(t('profiles.deleted'))
  } catch (e: any) {
    message.error(t('papers.failed', { error: e }))
  }
}

function onProfileSaved() {
  profileStore.fetchProfiles()
  editingProfile.value = undefined
}

watch(showProfileModal, (val) => {
  if (!val) editingProfile.value = undefined
})

// --- General ---
const currentLocale = ref(getLocale())
const localeOptions = computed(() => [
  { label: t('lang.en'), value: 'en' },
  { label: t('lang.ru'), value: 'ru' },
])

function switchLocale(value: string) {
  setLocale(value)
  currentLocale.value = value
}

function restartTour() {
  onboardingStore.restart()
  message.success(t('onboarding.restartTour'))
}

// --- Proxy ---
const proxyURL = ref('')
const testingProxy = ref(false)

// --- Semantic Scholar API key ---
const s2ApiKey = ref('')

async function loadSettings() {
  try {
    const settings = await GetSettings()
    proxyURL.value = settings['proxy_url'] || ''
    s2ApiKey.value = settings['semantic_scholar_api_key'] || ''
  } catch { /* ignore */ }
}

async function saveS2ApiKey() {
  try {
    await SaveSetting('semantic_scholar_api_key', s2ApiKey.value.trim())
    if (s2ApiKey.value.trim()) {
      message.success(t('settings.s2ApiKeySaved'))
    }
  } catch { /* ignore */ }
}

async function saveProxy() {
  try {
    await SaveSetting('proxy_url', proxyURL.value.trim())
    if (proxyURL.value.trim()) {
      message.success(t('settings.proxySaved'))
    }
  } catch { /* ignore */ }
}

async function testProxy() {
  testingProxy.value = true
  try {
    await TestProxy(proxyURL.value.trim())
    message.success(t('settings.proxyOk'))
  } catch (e: any) {
    message.error(t('settings.proxyFailed', { error: e }))
  } finally {
    testingProxy.value = false
  }
}

const checkingProviders = ref(false)
const providerStatus = ref<Array<{ name: string; ok: boolean; error: string; latency_ms: number }>>([])
async function checkProviders() {
  checkingProviders.value = true
  providerStatus.value = []
  try {
    providerStatus.value = await CheckSearchProviders()
    const okCount = providerStatus.value.filter(s => s.ok).length
    const total = providerStatus.value.length
    if (okCount === total) {
      message.success(t('settings.checkProvidersAllOk'))
    } else if (okCount === 0) {
      message.error(t('settings.checkProvidersAllFail'))
    } else {
      message.warning(t('settings.checkProvidersPartial', { ok: okCount, total }))
    }
  } catch (e: any) {
    message.error(e?.message || String(e))
  } finally {
    checkingProviders.value = false
  }
}

// --- Tags ---
const tags = ref<db.Tag[]>([])
const newTagName = ref('')
const newTagColor = ref('#6366f1')

const tagSwatches = [
  '#ef4444', '#f97316', '#eab308', '#22c55e',
  '#06b6d4', '#3b82f6', '#6366f1', '#a855f7',
  '#ec4899', '#64748b',
]

async function fetchTags() {
  const pid = profileStore.activeProfileId
  if (!pid) { tags.value = []; return }
  try { tags.value = await ListTags(pid) || [] }
  catch { tags.value = [] }
}

async function createTag() {
  const name = newTagName.value.trim()
  if (!name || !profileStore.activeProfileId) return
  try {
    await CreateTagAPI(new db.Tag({ profile_id: profileStore.activeProfileId, name, color: newTagColor.value }))
    newTagName.value = ''
    message.success(t('tags.created'))
    await fetchTags()
  } catch (e: any) {
    message.error(t('tags.createFailed', { error: e }))
  }
}

async function deleteTag(tag: db.Tag) {
  try {
    await DeleteTagAPI(tag.id)
    message.success(t('tags.deleted'))
    await fetchTags()
  } catch (e: any) {
    message.error(t('tags.deleteFailed', { error: e }))
  }
}

// --- Monitoring ---
const radarEnabled = ref(false)
const radarFrequency = ref('startup')
const runningRadar = computed(() => progressStore.radarRunning)

const radarFrequencyOptions = [
  { label: 'monitoring.frequencyStartup', value: 'startup' },
  { label: 'monitoring.frequency3h', value: '3h' },
  { label: 'monitoring.frequency6h', value: '6h' },
  { label: 'monitoring.frequency12h', value: '12h' },
  { label: 'monitoring.frequency24h', value: '24h' },
].map(opt => ({ ...opt, label: t(opt.label) }))

async function loadRadarSettings() {
  try {
    const settings = await GetSettings()
    radarEnabled.value = settings['enable_radar'] === 'true'
    radarFrequency.value = settings['radar_frequency'] || 'startup'
  } catch { /* ignore */ }
}

async function toggleRadar(value: boolean) {
  radarEnabled.value = value
  await SaveSetting('enable_radar', value ? 'true' : 'false')
  if (!value) {
    await SaveSetting('radar_frequency', 'startup')
    radarFrequency.value = 'startup'
  }
  message.success(t('monitoring.saved'))
}

async function saveRadarFrequency(value: string) {
  radarFrequency.value = value
  await SaveSetting('radar_frequency', value)
  message.success(t('monitoring.saved'))
}

async function runRadarNow() {
  const pid = profileStore.activeProfileId
  if (!pid) { message.warning(t('common.selectProfileFirst')); return }
  if (progressStore.radarRunning) return
  progressStore.radarRunning = true
  try {
    const count = await RunRadar(pid)
    if (count > 0) { message.success(t('monitoring.foundNew', { count })) }
    else { message.info(t('monitoring.foundNone')) }
  } catch (e: any) {
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(t('papers.failed', { error: e }))
    progressStore.radarRunning = false
  }
}

// --- LLM Profiles ---
const showLLMModal = ref(false)
const editingLLMProfile = ref<app.LLMProfileView | null>(null)
const testingId = ref<number | null>(null)

function openCreateProfile() {
  editingLLMProfile.value = null
  showLLMModal.value = true
}

function editProfileLLM(p: app.LLMProfileView) {
  editingLLMProfile.value = p
  showLLMModal.value = true
}

function onLLMProfileSaved() {
  editingLLMProfile.value = null
}

async function setActiveProfile(p: app.LLMProfileView) {
  try {
    await llmStore.setActive(p.id)
    message.success(t('llm.activated', { name: p.name }))
  } catch (e: any) {
    message.error(e?.message || String(e))
  }
}

async function deleteLLMProfile(p: app.LLMProfileView) {
  try {
    await llmStore.remove(p.id)
    message.success(t('llm.deleted'))
  } catch (e: any) {
    message.error(e?.message || String(e))
  }
}

async function testProfile(p: app.LLMProfileView) {
  testingId.value = p.id
  try {
    await llmStore.test(p.id)
    message.success(t('llm.testOk'))
  } catch (e: any) {
    message.error(e?.message || String(e))
  } finally {
    testingId.value = null
  }
}

watch(showLLMModal, (v) => { if (!v) editingLLMProfile.value = null })

async function loadPrompt() {
  try {
    const settings = await GetSettings()
    summaryPrompt.value = settings['llm_summary_prompt'] || ''
  } catch { /* ignore */ }
}

// --- Summary Prompt ---
const summaryPrompt = ref('')
const defaultPrompt = `You are an academic research assistant. Analyze the provided scientific paper and produce a structured summary in Markdown format:

## Main Idea
One paragraph summarizing the core contribution.

## Method
Brief description of methodology/approach.

## Key Findings
- Bullet points of main results

## Limitations
- Known limitations mentioned by authors

## Relevance
How this paper relates to the broader field. What gap does it fill?

Keep the summary concise (300-500 words). Use the language of the paper.`

async function savePrompt() {
  await SaveSetting('llm_summary_prompt', summaryPrompt.value)
  message.success(t('settings.prompt.saved'))
}

function resetPrompt() {
  summaryPrompt.value = defaultPrompt
  savePrompt()
}

const appVersion = ref('')

onMounted(() => {
  loadSettings()
  loadRadarSettings()
  loadPrompt()
  llmStore.fetch()
  fetchTags()
  AppVersion().then((v) => { appVersion.value = v }).catch(() => {})
})

watch(() => profileStore.activeProfileId, () => { fetchTags() })
</script>

<style scoped>
.settings-layout {
  display: flex;
  gap: var(--space-8, 32px);
}

.settings-content {
  flex: 1;
  min-width: 0;
}

.settings-content :deep(.n-h3) {
  font-size: var(--text-h3, 18px);
  font-weight: var(--weight-semibold, 600);
  color: var(--text-primary, #f8fafc);
}

.active-profile {
  border-color: var(--color-primary-500, #7c5cff);
  background: rgba(124, 92, 255, 0.04);
}
.color-swatch-trigger {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-md, 8px);
  cursor: pointer;
  border: 2px solid var(--surface-border, rgba(148, 163, 184, 0.12));
  transition: border-color var(--transition-fast, 150ms ease);
}
.color-swatch-trigger:hover {
  border-color: var(--text-tertiary, rgba(148, 163, 184, 0.4));
}
.color-swatch-option {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  cursor: pointer;
  border: 2px solid transparent;
  transition: transform 0.15s, border-color 0.15s;
}
.color-swatch-option:hover {
  transform: scale(1.15);
}
.color-swatch-option.active {
  border-color: var(--text-primary, #f8fafc);
  box-shadow: 0 0 0 2px currentColor;
}
</style>
