import { createI18n, type I18n } from "vue-i18n"
import { loadLocaleMessages, type Locale } from "../locales"

export let i18n: I18n

const FALLBACK_LOCALE: Locale = "en-US"

export async function initI18n(locale: Locale) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const msgs: Record<string, any> = {}

  const targets = locale === FALLBACK_LOCALE ? [locale] : [locale, FALLBACK_LOCALE]
  const loaded = await Promise.all(targets.map((l) => loadLocaleMessages(l)))
  targets.forEach((l, idx) => {
    msgs[l] = loaded[idx]
  })

  i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: FALLBACK_LOCALE,
    messages: msgs,
  })
  return i18n
}
