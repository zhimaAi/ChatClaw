import { BrowserService } from '@bindings/chatclaw/internal/services/browser'

const DEFAULT_PLATFORM_DOCS_URL = 'https://docs.ichatclaw.com/docs/chatClaw-access-to-feishu'

const PLATFORM_DOCS_URLS: Record<string, string> = {
  feishu: 'https://docs.ichatclaw.com/docs/chatClaw-access-to-feishu',
  wecom: 'https://docs.ichatclaw.com/docs/chatClaw-access-to-work-weixin-robot',
  dingtalk: 'https://docs.ichatclaw.com/docs/chatClaw-access-to-dingtalk',
  qq: 'https://docs.ichatclaw.com/docs/chatClaw-access-to-qq-robot',
}

export function getPlatformDocsUrl(platformId?: string | null): string {
  if (!platformId) return DEFAULT_PLATFORM_DOCS_URL
  return PLATFORM_DOCS_URLS[platformId] || DEFAULT_PLATFORM_DOCS_URL
}

export async function openExternalLink(url: string) {
  try {
    await BrowserService.OpenURL(url)
  } catch (error) {
    console.error('Failed to open external link:', error)
  }
}
