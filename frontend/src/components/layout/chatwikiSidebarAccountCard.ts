export type ChatwikiSidebarBindingLike = {
  chatwiki_version?: string | null
  user_name?: string | null
  user_id?: string | null
}

export type ChatwikiSidebarCatalogLike = {
  integral_stats?: {
    raw?: unknown
  } | null
}

export type ChatwikiSidebarAccountCardState =
  | {
      mode: 'login'
      action: 'login'
      accountLabel: ''
      creditsLabel: '立即登录'
    }
  | {
      mode: 'bound'
      action: 'openProviderSettings'
      accountLabel: string
      creditsLabel: string
    }

function normalizeText(value?: string | null): string {
  return value?.trim() || ''
}

function findDeepStringValue(input: unknown, key: string): string {
  if (!input || typeof input !== 'object') return ''

  if (Array.isArray(input)) {
    for (const item of input) {
      const nested = findDeepStringValue(item, key)
      if (nested) return nested
    }
    return ''
  }

  const record = input as Record<string, unknown>
  if (record[key] != null) {
    return String(record[key])
  }

  for (const value of Object.values(record)) {
    const nested = findDeepStringValue(value, key)
    if (nested) return nested
  }

  return ''
}

function extractStatValue(catalog: ChatwikiSidebarCatalogLike | null, key: string): string {
  const raw = catalog?.integral_stats?.raw
  if (!raw) return ''

  try {
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
    return findDeepStringValue(parsed, key)
  } catch {
    return ''
  }
}

function formatCreditsLabel(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return '-- 积分'

  const num = Number(trimmed)
  if (Number.isNaN(num)) {
    return `${trimmed} 积分`
  }

  return `${num.toLocaleString('zh-CN', { maximumFractionDigits: 3 })} 积分`
}

export function isChatwikiCloudBinding(binding: ChatwikiSidebarBindingLike | null): boolean {
  if (!binding) return false
  const version = normalizeText(binding.chatwiki_version).toLowerCase()
  return !!version && version !== 'dev'
}

export function shouldAutoRefreshChatwikiCredits(
  binding: ChatwikiSidebarBindingLike | null
): boolean {
  return isChatwikiCloudBinding(binding)
}

export function getChatwikiCreditsRefreshMode(mode: 'initial' | 'polling'): boolean {
  return mode === 'polling'
}

export function buildChatwikiSidebarAccountCardState(
  binding: ChatwikiSidebarBindingLike | null,
  catalog: ChatwikiSidebarCatalogLike | null
): ChatwikiSidebarAccountCardState {
  if (!isChatwikiCloudBinding(binding)) {
    return {
      mode: 'login',
      action: 'login',
      accountLabel: '',
      creditsLabel: '立即登录',
    }
  }

  const accountLabel =
    normalizeText(binding?.user_name) || normalizeText(binding?.user_id) || 'ChatWiki'

  return {
    mode: 'bound',
    action: 'openProviderSettings',
    accountLabel,
    creditsLabel: formatCreditsLabel(extractStatValue(catalog, 'all_surplus')),
  }
}
