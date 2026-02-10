import { computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { type Locale } from '../locales'
import { Service as I18nService } from '@bindings/willclaw/internal/services/i18n'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'

const DEFAULT_LOCALE: Locale = 'zh-CN'

/**
 * 从后端获取语言配置
 */
export async function fetchLocale(): Promise<Locale> {
  try {
    // Prefer persisted settings value
    const s = await SettingsService.Get('language')
    const v = (s?.value || '').trim()
    if (v === 'zh-CN' || v === 'en-US') {
      // keep backend localizer consistent
      try {
        await I18nService.SetLocale(v)
      } catch {
        // ignore
      }
      return v
    }

    const locale = await I18nService.GetLocale()
    if (locale === 'zh-CN' || locale === 'en-US') {
      return locale
    }
  } catch (e) {
    console.warn('Failed to fetch locale from backend:', e)
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
    if (newLocale && (newLocale === 'zh-CN' || newLocale === 'en-US')) {
      i18n.value = newLocale
    }
  })

  return unsubscribe
}
