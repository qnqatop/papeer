<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <AppShell />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { darkTheme, NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import { themeOverrides } from './theme/naive-overrides'
import AppShell from './AppShell.vue'
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
