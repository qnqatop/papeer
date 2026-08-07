import { createI18n } from 'vue-i18n'
import { registerMessageCompiler, compile, registerMessageResolver, resolveValue, registerLocaleFallbacker, fallbackWithLocaleChain } from '@intlify/core-base'
import en from './en.json'
import ru from './ru.json'

// vue-i18n ships with sideEffects:false, so Rollup can tree-shake the
// implicit compiler/resolver/fallbacker registration in production.
// Registering them explicitly guarantees translations work in the build.
registerMessageCompiler(compile)
registerMessageResolver(resolveValue)
registerLocaleFallbacker(fallbackWithLocaleChain)

const LOCALE_KEY = 'papeer-locale'
const LEGACY_LOCALE_KEY = 'apepa-locale' // pre-rebrand, migrated on first read

// Migrate pre-rebrand key once, then read the current one.
if (localStorage.getItem(LEGACY_LOCALE_KEY) !== null) {
  if (localStorage.getItem(LOCALE_KEY) === null) {
    localStorage.setItem(LOCALE_KEY, localStorage.getItem(LEGACY_LOCALE_KEY) as string)
  }
  localStorage.removeItem(LEGACY_LOCALE_KEY)
}

const savedLocale = localStorage.getItem(LOCALE_KEY) || 'ru'

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'en',
  messages: { en, ru },
})

export default i18n

export function setLocale(locale: string) {
  ;(i18n.global.locale as any).value = locale
  localStorage.setItem(LOCALE_KEY, locale)
}

export function getLocale(): string {
  return (i18n.global.locale as any).value
}
