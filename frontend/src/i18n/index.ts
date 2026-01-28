import { createI18n, type I18n } from 'vue-i18n'
import { messages, type Locale } from '../locales'

// i18n 实例（由 initI18n 初始化）
export let i18n: I18n

/**
 * 初始化 i18n 实例（由后端指定语言）
 */
export function initI18n(locale: Locale) {
  i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'en-US',
    messages,
  })
  return i18n
}
