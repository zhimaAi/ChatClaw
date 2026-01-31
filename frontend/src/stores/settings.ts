import { ref } from 'vue'
import { defineStore } from 'pinia'

/**
 * 设置菜单项类型
 */
export type SettingsMenuItem =
  | 'modelService'
  | 'generalSettings'
  | 'snapSettings'
  | 'tools'
  | 'about'

/**
 * 设置页面状态管理
 */
export const useSettingsStore = defineStore('settings', () => {
  // 当前激活的菜单项
  const activeMenu = ref<SettingsMenuItem>('generalSettings')

  /**
   * 设置当前激活的菜单项
   */
  const setActiveMenu = (menu: SettingsMenuItem) => {
    activeMenu.value = menu
  }

  return {
    activeMenu,
    setActiveMenu,
  }
})
