export const LOCALE_KEYS = [
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
] as const

export type Locale = (typeof LOCALE_KEYS)[number]

const loaders: Record<Locale, () => Promise<{ default: Record<string, unknown> }>> = {
  'zh-CN': () => import('./zh-CN'),
  'en-US': () => import('./en-US'),
  'ar-SA': () => import('./ar-SA'),
  'bn-BD': () => import('./bn-BD'),
  'de-DE': () => import('./de-DE'),
  'es-ES': () => import('./es-ES'),
  'fr-FR': () => import('./fr-FR'),
  'hi-IN': () => import('./hi-IN'),
  'it-IT': () => import('./it-IT'),
  'ja-JP': () => import('./ja-JP'),
  'ko-KR': () => import('./ko-KR'),
  'pt-BR': () => import('./pt-BR'),
  'sl-SI': () => import('./sl-SI'),
  'tr-TR': () => import('./tr-TR'),
  'vi-VN': () => import('./vi-VN'),
  'zh-TW': () => import('./zh-TW'),
}

const cache = new Map<Locale, Record<string, unknown>>()

export async function loadLocaleMessages(locale: Locale): Promise<Record<string, unknown>> {
  const cached = cache.get(locale)
  if (cached) return cached

  const mod = await loaders[locale]()
  const messages = mod.default
  cache.set(locale, messages)
  return messages
}
