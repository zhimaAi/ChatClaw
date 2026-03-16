import { computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { LOCALE_KEYS, loadLocaleMessages, type Locale } from '../locales'
import { i18n } from '../i18n'
import { Service as I18nService } from '@bindings/chatclaw/internal/services/i18n'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'

// Re-export Locale type so consumers can import from this composable
export type { Locale } from '../locales'

const DEFAULT_LOCALE: Locale = 'en-US'

export const SUPPORTED_LOCALES: readonly Locale[] = LOCALE_KEYS

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

async function ensureLocaleLoaded(locale: Locale) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const global = i18n.global as any
  if (!global.availableLocales.includes(locale)) {
    const messages = await loadLocaleMessages(locale)
    global.setLocaleMessage(locale, messages)
  }
}

/**
 * 组件内使用的 locale composable
 */
export function useLocale() {
  const { locale } = useI18n()

  const currentLocale = computed(() => locale.value as Locale)

  async function switchLocale(newLocale: Locale) {
    await ensureLocaleLoaded(newLocale)
    await Promise.all([
      SettingsService.SetValue('language', newLocale),
      I18nService.SetLocale(newLocale),
    ])
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
  const localeValue = localeRef ?? useI18n().locale

  const unsubscribe = Events.On('locale:changed', async (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const newLocale = payload?.locale
    if (newLocale && SUPPORTED_LOCALES.includes(newLocale as Locale)) {
      await ensureLocaleLoaded(newLocale as Locale)
      localeValue.value = newLocale
    }
  })

  return unsubscribe
}
