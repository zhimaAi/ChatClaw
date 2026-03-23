import { ref } from 'vue'
import { defineStore } from 'pinia'

/**
 * 设置菜单项类型
 */
export type SettingsMenuItem =
  | 'modelService'
  | 'generalSettings'
  | 'openclawRuntime'
  | 'skills'
  | 'mcp'
  | 'snapSettings'
  | 'tools'
  | 'chatwiki'
  | 'about'

export type PendingChatwikiAction = 'cloudLogin'

/**
 * 设置页面状态管理
 */
export const useSettingsStore = defineStore('settings', () => {
  const activeMenu = ref<SettingsMenuItem>('generalSettings')
  const pendingChatwikiAction = ref<PendingChatwikiAction | null>(null)
  const pendingModelServiceProviderId = ref<string | null>(null)

  const setActiveMenu = (menu: SettingsMenuItem) => {
    activeMenu.value = menu
  }

  const requestChatwikiCloudLogin = () => {
    pendingChatwikiAction.value = 'cloudLogin'
  }

  const requestModelServiceProviderSelection = (providerId: string) => {
    pendingModelServiceProviderId.value = providerId
  }

  const consumePendingChatwikiAction = (): PendingChatwikiAction | null => {
    const nextAction = pendingChatwikiAction.value
    pendingChatwikiAction.value = null
    return nextAction
  }

  const consumePendingModelServiceProviderId = (): string | null => {
    const nextProviderId = pendingModelServiceProviderId.value
    pendingModelServiceProviderId.value = null
    return nextProviderId
  }

  return {
    activeMenu,
    pendingChatwikiAction,
    pendingModelServiceProviderId,
    setActiveMenu,
    requestChatwikiCloudLogin,
    requestModelServiceProviderSelection,
    consumePendingChatwikiAction,
    consumePendingModelServiceProviderId,
  }
})
