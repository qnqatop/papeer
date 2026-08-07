import '@fontsource-variable/inter'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import './style.css'

const app = createApp(App)
app.use(createPinia())
app.use(i18n)
app.use(router)

// Global error handler — catches unhandled errors from Wails calls and Vue components.
app.config.errorHandler = (err, _instance, info) => {
  console.error(`[papeer] ${info}:`, err)
}

window.addEventListener('unhandledrejection', (event) => {
  console.error('[papeer] Unhandled promise rejection:', event.reason)
})

app.mount('#app')
