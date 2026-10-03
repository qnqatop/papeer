import { describe, it, expect } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createMemoryHistory } from 'vue-router'
import { defineComponent } from 'vue'
import S2RateLimitHint from '../components/S2RateLimitHint.vue'
import ru from '../i18n/ru.json'

const Stub = defineComponent({ template: '<div />' })

function setup() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'search', component: Stub },
      { path: '/settings', name: 'settings', component: Stub },
    ],
  })
  const i18n = createI18n({ legacy: false, locale: 'ru', messages: { ru } })
  const wrapper = mount(S2RateLimitHint, { global: { plugins: [router, i18n] } })
  return { router, wrapper }
}

describe('S2RateLimitHint', () => {
  it('shows the localized API-key hint', () => {
    const { wrapper } = setup()
    expect(wrapper.text()).toContain('Semantic Scholar ограничил запросы — добавьте API-ключ в Настройки → Поиск')
  })

  it('navigates to Settings → Search and tells the host to close', async () => {
    const { router, wrapper } = setup()
    await router.push('/')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('settings')
    expect(router.currentRoute.value.query.section).toBe('search')
    expect(wrapper.emitted('navigate')).toHaveLength(1)
  })
})
