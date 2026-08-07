<template>
  <n-modal v-model:show="visible" preset="card" :title="editingId ? t('profiles.editProfile') : t('profiles.createProfile')" style="width: 500px">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="120">
      <n-form-item :label="t('profiles.name')" path="name">
        <n-input v-model:value="form.name" :placeholder="t('profiles.namePlaceholder')" />
      </n-form-item>
      <n-form-item path="email" required>
        <template #label>
          <span class="label-with-hint">{{ t('profiles.email') }}<n-tooltip><template #trigger><n-icon :component="HelpCircleOutline" size="14" class="label-hint-icon" /></template>{{ t('profiles.emailHint') }}</n-tooltip></span>
        </template>
        <n-input v-model:value="form.email" :placeholder="t('profiles.emailPlaceholder')" @blur="formRef?.validate(undefined, (r: any) => r.key === 'email')" />
      </n-form-item>
      <n-form-item :label="t('profiles.pdfDir')">
        <n-input-group>
          <n-input v-model:value="form.pdf_dir" :placeholder="t('profiles.pdfDirPlaceholder')" readonly />
          <n-button @click="pickDir">{{ t('common.browse') }}</n-button>
        </n-input-group>
      </n-form-item>
      <n-form-item>
        <template #label>
          <span class="label-with-hint">{{ t('profiles.yearMin') }}<n-tooltip><template #trigger><n-icon :component="HelpCircleOutline" size="14" class="label-hint-icon" /></template>{{ t('profiles.yearMinHint') }}</n-tooltip></span>
        </template>
        <n-input-number v-model:value="form.year_min" :min="2000" :max="2030" />
      </n-form-item>
    </n-form>

    <template #action>
      <n-space justify="end">
        <n-button @click="visible = false">{{ t('common.cancel') }}</n-button>
        <n-button type="primary" :loading="saving" @click="saveProfile">
          {{ editingId ? t('common.save') : t('common.create') }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NModal, NForm, NFormItem, NInput, NInputNumber,
  NInputGroup, NButton, NSpace, NTooltip, NIcon,
  useMessage,
} from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { HelpCircleOutline } from '@vicons/ionicons5'
import { useProfileStore } from '../stores/profile'
import { useOnboardingStore } from '../stores/onboarding'
import { isRealEmail } from '../utils/validate'
import { db } from '../../wailsjs/go/models'

const { t } = useI18n()
const profileStore = useProfileStore()
const onboardingStore = useOnboardingStore()
const message = useMessage()

const visible = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  profile?: db.Profile | null
  initial?: Partial<{ name: string; email: string; pdf_dir: string; year_min: number; max_per_query: number }> | null
}>()

const emit = defineEmits<{
  saved: [profile: db.Profile]
}>()

const editingId = ref<number | null>(null)
const saving = ref(false)
const formRef = ref<FormInst | null>(null)

const form = reactive({
  name: '',
  email: '',
  pdf_dir: '',
  year_min: 2018,
  max_per_query: 25,
})

const rules: FormRules = {
  email: [
    {
      required: true,
      trigger: ['blur', 'input'],
      validator(_rule, value: string) {
        const s = (value || '').trim()
        if (!s) return new Error(t('profiles.emailRequired'))
        if (!isRealEmail(s)) return new Error(t('profiles.emailInvalid'))
        return true
      },
    },
  ],
}

watch(visible, (v) => {
  if (v) {
    if (props.profile) {
      editingId.value = props.profile.id
      Object.assign(form, {
        name: props.profile.name,
        email: props.profile.email,
        pdf_dir: props.profile.pdf_dir,
        year_min: props.profile.year_min || 2018,
        max_per_query: props.profile.max_per_query || 25,
      })
    } else {
      editingId.value = null
      Object.assign(form, { name: '', email: '', pdf_dir: '', year_min: 2018, max_per_query: 25 })
      if (props.initial) Object.assign(form, props.initial)
    }
  }
})

async function pickDir() {
  try {
    const dir = await profileStore.selectPdfDir()
    if (dir) form.pdf_dir = dir
  } catch (e: any) {
    message.error(e.message || t('profiles.selectDirFailed'))
  }
}

async function saveProfile() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      const p = new db.Profile({ id: editingId.value, ...form })
      await profileStore.updateProfile(p)
      message.success(t('profiles.updated'))
      emit('saved', p)
    } else {
      const created = await profileStore.createProfile(form)
      message.success(t('profiles.created'))
      onboardingStore.markPhaseComplete(1)
      emit('saved', created)
    }
    visible.value = false
  } catch (e: any) {
    message.error(e.message || t('profiles.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.label-with-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.label-hint-icon {
  cursor: help;
  opacity: 0.4;
  flex-shrink: 0;
}
</style>
