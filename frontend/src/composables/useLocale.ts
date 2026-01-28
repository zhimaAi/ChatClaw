import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { type Locale } from '../locales'
import { I18nService } from '../wails'

const DEFAULT_LOCALE: Locale = 'zh-CN'

/**
 * 从后端获取语言配置
 */
export async function fetchLocale(): Promise<Locale> {
  try {
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
    // 更新后端
    await I18nService.SetLocale(newLocale)
    // 更新前端
    locale.value = newLocale
  }

  return {
    locale: currentLocale,
    switchLocale,
  }
}
