import { ref } from 'vue'
import { defineStore } from 'pinia'

/**
 * 设置菜单项类型
 */
export type SettingsMenuItem =
  | 'modelService'
  | 'generalSettings'
  | 'memorySettings'
  | 'skills'
  | 'snapSettings'
  | 'tools'
  | 'chatwiki'
  | 'about'

/**
 * 设置页面状态管理
 */
export const useSettingsStore = defineStore('settings', () => {
  const activeMenu = ref<SettingsMenuItem>('generalSettings')

  const setActiveMenu = (menu: SettingsMenuItem) => {
    activeMenu.value = menu
  }

  return {
    activeMenu,
    setActiveMenu,
  }
})
