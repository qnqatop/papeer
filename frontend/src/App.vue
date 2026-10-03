<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <AppShell />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { darkTheme, NConfigProvider, NMessageProvider, NDialogProvider, useOsTheme } from 'naive-ui'
import { getThemeOverrides } from './theme/naive-overrides'
import { themePreference, resolvedThemeMode, resolveThemeMode } from './theme/mode'
import AppShell from './AppShell.vue'

const osTheme = useOsTheme()
const mode = computed(() => resolveThemeMode(themePreference.value, osTheme.value))
// null selects Naive UI's built-in light theme.
const naiveTheme = computed(() => (mode.value === 'dark' ? darkTheme : null))
const themeOverrides = computed(() => getThemeOverrides(mode.value))

// Expose the mode to plain CSS (tokens.css [data-theme="light"]) and to
// components that need concrete colors (graphs).
watchEffect(() => {
  resolvedThemeMode.value = mode.value
  document.documentElement.dataset.theme = mode.value
  document.documentElement.style.colorScheme = mode.value
})
</script>

<style>
@import './theme/tokens.css';

body {
  margin: 0;
  padding: 0;
  font-family: var(--font-family);
  background-color: var(--surface-bg);
  color: var(--text-primary);
}
</style>
