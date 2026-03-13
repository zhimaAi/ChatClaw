import { computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { type Locale } from '../locales'
import { Service as I18nService } from '@bindings/chatclaw/internal/services/i18n'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'

// Re-export Locale type so consumers can import from this composable
export type { Locale } from '../locales'

const DEFAULT_LOCALE: Locale = 'en-US'

// 支持的语言列表
export const SUPPORTED_LOCALES: Locale[] = [
  'zh-CN',
  'en-US',
  'ar-SA',
  'bn-BD',
  'de-DE',
  'es-ES',
  'fr-FR',
  'hi-IN',
  'it-IT',
  'ja-JP',
  'ko-KR',
  'pt-BR',
  'sl-SI',
  'tr-TR',
  'vi-VN',
  'zh-TW',
]

function detectSystemLocale(): Locale | null {
  if (typeof navigator === 'undefined') {
    return null
  }

  const candidates: string[] = []
  const primary =
    navigator.language ||
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (navigator as any).userLanguage
  if (primary) {
    candidates.push(primary)
  }
  if (Array.isArray(navigator.languages)) {
    candidates.push(...navigator.languages)
  }

  for (const raw of candidates) {
    if (!raw) continue
    const normalized = raw.trim().replace('_', '-')
    if (!normalized) continue

    const lower = normalized.toLowerCase()
    const fullMatch = SUPPORTED_LOCALES.find(
      (l) => l.toLowerCase() === lower,
    )
    if (fullMatch) {
      return fullMatch
    }

    const base = lower.split('-')[0]
    if (!base) continue
    const baseMatch = SUPPORTED_LOCALES.find((l) =>
      l.toLowerCase().startsWith(`${base}-`),
    )
    if (baseMatch) {
      return baseMatch
    }
  }

  return null
}

/**
 * 从后端获取语言配置
 */
export async function fetchLocale(): Promise<Locale> {
  try {
    const s = await SettingsService.Get('language')
    const v = (s?.value || '').trim()
    if (SUPPORTED_LOCALES.includes(v as Locale)) {
      try {
        await I18nService.SetLocale(v)
      } catch {
        // ignore
      }
      return v as Locale
    }

    const locale = await I18nService.GetLocale()
    if (SUPPORTED_LOCALES.includes(locale as Locale)) {
      return locale as Locale
    }
  } catch (e) {
    console.warn('Failed to fetch locale from backend:', e)
  }

  const systemLocale = detectSystemLocale()
  if (systemLocale) {
    try {
      await I18nService.SetLocale(systemLocale)
    } catch {
      // ignore
    }
    return systemLocale
  }

  return DEFAULT_LOCALE
}

/**
 * 组件内使用的 locale composable
 */
export function useLocale() {
  const { locale } = useI18n()

  // 当前语言（响应式）
  const currentLocale = computed(() => locale.value as Locale)

  // 切换语言（同时更新前端和后端）
  async function switchLocale(newLocale: Locale) {
    // Persist to DB and update backend localizer.
    // I18nService.SetLocale also emits 'locale:changed' from Go backend
    // so all windows (snap, selection) will be notified.
    await Promise.all([
      SettingsService.SetValue('language', newLocale),
      I18nService.SetLocale(newLocale),
    ])
    // 更新当前窗口前端
    locale.value = newLocale
  }

  return {
    locale: currentLocale,
    switchLocale,
  }
}

/**
 * Listen for locale changes broadcast from Go backend and sync i18n.
 * Call once in each window's App.vue setup (snap, selection, etc.).
 * Returns an unsubscribe function.
 */
export function useLocaleSync(localeRef?: Ref<string>) {
  const i18n = localeRef ?? useI18n().locale

  const unsubscribe = Events.On('locale:changed', (event: any) => {
    // Go backend emits map[string]string{"locale": "xx"}.
    // Wails wraps it as event.data (object or array).
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const newLocale = payload?.locale
    if (newLocale && SUPPORTED_LOCALES.includes(newLocale as Locale)) {
      i18n.value = newLocale
    }
  })

  return unsubscribe
}
