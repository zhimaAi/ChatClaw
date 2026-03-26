import { platformIconMap } from '@/assets/icons/snap/platformIcons'

export function getPlatformIcon(platformId: string): string | null {
  return platformIconMap[platformId] || null
}

export function getPlatformIconBg(platformId: string): string {
  switch (platformId) {
    case 'wecom':
      return '#E6FFE6'
    case 'qq':
    case 'dingtalk':
      return '#E5F3FF'
    case 'feishu':
      return '#E1FAF7'
    default:
      return '#F5F5F5'
  }
}

export function getAppIdFromConfig(extraConfig: string, fallback = 'N/A'): string {
  try {
    const config = JSON.parse(extraConfig)
    return config.app_id || config.token || fallback
  } catch {
    return fallback
  }
}
