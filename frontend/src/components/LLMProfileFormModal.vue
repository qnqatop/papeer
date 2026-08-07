<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="editingId ? t('llm.editProfile') : t('llm.createProfile')"
    style="width: 560px"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item :label="t('llm.name')" path="name">
        <n-input v-model:value="form.name" :placeholder="t('llm.namePlaceholder')" />
      </n-form-item>

      <n-form-item :label="t('llm.preset')">
        <n-select v-model:value="preset" :options="presetOptions" @update:value="applyPreset" />
      </n-form-item>
      <n-text depth="3" style="font-size: 12px; margin: -8px 0 12px">{{ t('llm.presetHint') }}</n-text>

      <n-form-item :label="t('llm.baseUrl')" path="base_url">
        <n-input v-model:value="form.base_url" placeholder="https://api.deepseek.com/v1" />
      </n-form-item>

      <n-form-item :label="t('llm.apiKey')">
        <n-input
          v-model:value="apiKey"
          type="password"
          show-password-on="click"
          :placeholder="editingId ? t('llm.apiKeyKeepPlaceholder') : t('llm.apiKeyPlaceholder')"
        />
      </n-form-item>
      <n-text depth="3" style="font-size: 12px; margin: -8px 0 12px">🔒 {{ t('llm.apiKeyHint') }}</n-text>

      <n-form-item :label="t('llm.models')" path="models">
        <n-dynamic-tags v-model:value="form.models" @update:value="onModelsChange" />
      </n-form-item>
      <n-text depth="3" style="font-size: 12px; margin: -8px 0 12px">{{ t('llm.modelsHint') }}</n-text>

      <n-form-item :label="t('llm.defaultModel')" path="default_model">
        <n-select
          v-model:value="form.default_model"
          :options="modelOptions"
          :placeholder="t('llm.defaultModelPlaceholder')"
        />
      </n-form-item>

      <n-form-item :label="t('llm.temperature', { value: form.temperature.toFixed(1) })">
        <n-slider v-model:value="form.temperature" :min="0" :max="1" :step="0.1" />
      </n-form-item>

      <n-form-item>
        <n-checkbox v-model:checked="form.is_active">{{ t('llm.makeActive') }}</n-checkbox>
      </n-form-item>
    </n-form>

    <!-- Test connection status -->
    <n-space v-if="testState !== 'idle'" align="center" :size="8" style="margin-top: 4px">
      <n-spin v-if="testState === 'testing'" size="small" />
      <n-icon v-else-if="testState === 'ok'" :component="CheckmarkCircle" color="#10b981" />
      <n-icon v-else :component="CloseCircle" color="#ef4444" />
      <n-text :depth="testState === 'error' ? undefined : 3" :type="testState === 'error' ? 'error' : undefined" style="font-size: 13px">
        {{ testMessage }}
      </n-text>
    </n-space>

    <template #action>
      <n-space justify="space-between">
        <n-button :loading="testState === 'testing'" :disabled="!canTest" @click="testConnection">
          {{ t('llm.testConnection') }}
        </n-button>
        <n-space>
          <n-button @click="visible = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</n-button>
        </n-space>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NModal, NForm, NFormItem, NInput, NSelect, NSlider, NDynamicTags,
  NCheckbox, NButton, NSpace, NText, NSpin, NIcon,
  useMessage,
} from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { CheckmarkCircle, CloseCircle } from '@vicons/ionicons5'
import { useLLMProfilesStore } from '../stores/llmProfiles'
import { app, db } from '../../wailsjs/go/models'

const { t } = useI18n()
const store = useLLMProfilesStore()
const message = useMessage()

const visible = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  profile?: app.LLMProfileView | null
}>()

const emit = defineEmits<{
  saved: []
}>()

const editingId = ref<number | null>(null)
const saving = ref(false)
const formRef = ref<FormInst | null>(null)
const apiKey = ref('')
const preset = ref<'deepseek' | 'custom'>('custom')

const form = reactive({
  name: '',
  base_url: '',
  models: [] as string[],
  default_model: '',
  temperature: 0.2,
  is_active: false,
})

const presetOptions = computed(() => [
  { label: 'DeepSeek', value: 'deepseek' },
  { label: t('llm.presetCustom'), value: 'custom' },
])

const modelOptions = computed(() =>
  form.models.map(m => ({ label: m, value: m })),
)

const rules = computed<FormRules>(() => ({
  name: [{ required: true, trigger: ['blur', 'input'], validator: (_r, v: string) => (v || '').trim() ? true : new Error(t('llm.nameRequired')) }],
  base_url: [{ required: true, trigger: ['blur', 'input'], validator: (_r, v: string) => (v || '').trim() ? true : new Error(t('llm.baseUrlRequired')) }],
  models: [{ validator: () => form.models.length > 0 ? true : new Error(t('llm.modelsRequired')), trigger: ['change'] }],
  default_model: [{ validator: () => form.default_model ? true : new Error(t('llm.defaultModelRequired')), trigger: ['change'] }],
}))

const canTest = computed(() =>
  !!form.base_url.trim() && !!form.default_model &&
  (!!apiKey.value.trim() || (editingId.value !== null && (props.profile?.has_key ?? false))),
)

type TestState = 'idle' | 'testing' | 'ok' | 'error'
const testState = ref<TestState>('idle')
const testMessage = ref('')

function applyPreset(value: 'deepseek' | 'custom') {
  if (value === 'deepseek') {
    form.base_url = 'https://api.deepseek.com/v1'
    if (form.models.length === 0) {
      form.models = ['deepseek-chat', 'deepseek-reasoner']
      form.default_model = 'deepseek-chat'
    }
  }
}

function onModelsChange(models: string[]) {
  if (!models.includes(form.default_model)) {
    form.default_model = models[0] || ''
  }
}

async function testConnection() {
  testState.value = 'testing'
  testMessage.value = t('llm.testInProgress')
  try {
    await store.testDraft(form.base_url.trim(), apiKey.value.trim(), form.default_model)
    testState.value = 'ok'
    testMessage.value = t('llm.testOk')
  } catch (e: any) {
    testState.value = 'error'
    testMessage.value = e?.message || String(e)
  }
}

async function save() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    const payload = new db.LLMProfile({
      id: editingId.value || 0,
      name: form.name.trim(),
      base_url: form.base_url.trim(),
      models: [...form.models],
      default_model: form.default_model,
      temperature: form.temperature,
      is_active: form.is_active,
    })
    if (editingId.value) {
      await store.update(payload, apiKey.value.trim())
      message.success(t('llm.updated'))
    } else {
      await store.create(payload, apiKey.value.trim())
      message.success(t('llm.created'))
    }
    emit('saved')
    visible.value = false
  } catch (e: any) {
    message.error(e?.message || String(e))
  } finally {
    saving.value = false
  }
}

watch(visible, (v) => {
  if (!v) return
  apiKey.value = ''
  testState.value = 'idle'
  testMessage.value = ''
  if (props.profile) {
    editingId.value = props.profile.id
    Object.assign(form, {
      name: props.profile.name,
      base_url: props.profile.base_url,
      models: [...(props.profile.models || [])],
      default_model: props.profile.default_model,
      temperature: props.profile.temperature ?? 0.2,
      is_active: props.profile.is_active,
    })
    preset.value = props.profile.base_url === 'https://api.deepseek.com/v1' ? 'deepseek' : 'custom'
  } else {
    editingId.value = null
    preset.value = 'custom'
    Object.assign(form, { name: '', base_url: '', models: [], default_model: '', temperature: 0.2, is_active: false })
  }
})
</script>
