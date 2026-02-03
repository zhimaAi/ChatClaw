import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import DefaultTabIcon from '@/assets/images/tab-default.png'

const createTabId = () => `tab-${crypto.randomUUID()}`

/**
 * 导航模块类型
 */
export type NavModule = 'assistant' | 'knowledge' | 'multiask' | 'settings'

/**
 * 标签页类型
 */
export interface Tab {
  id: string
  /** 标签页标题（自定义标题时使用） */
  title?: string
  /** 标签页标题的翻译键（用于动态翻译） */
  titleKey?: string
  /** 标签页图标URL */
  icon?: string
  /** 关联的模块 */
  module: NavModule
}

/**
 * 模块标题映射（用于创建标签页时的默认标题）
 */
const moduleLabels: Record<NavModule, string> = {
  assistant: 'nav.assistant',
  knowledge: 'nav.knowledge',
  multiask: 'nav.multiask',
  settings: 'nav.settings',
}

/**
 * 只允许单个标签页的模块列表
 * 这些模块点击时如果已存在标签页，则切换到该标签页而不是新建
 */
const singleTabModules: NavModule[] = ['knowledge', 'multiask', 'settings']

/**
 * 导航状态管理
 * 管理左侧导航菜单和顶部标签页
 */
export const useNavigationStore = defineStore('navigation', () => {
  // 当前激活的导航模块
  const activeModule = ref<NavModule>('assistant')

  // 侧边栏是否折叠
  const sidebarCollapsed = ref(false)

  // 标签页列表
  const tabs = ref<Tab[]>([])

  // 当前激活的标签页ID
  const activeTabId = ref<string | null>(null)

  // 当前激活的标签页
  const activeTab = computed(() => {
    if (!activeTabId.value) return null
    return tabs.value.find((tab) => tab.id === activeTabId.value) || null
  })

  /**
   * 切换侧边栏折叠状态
   */
  const toggleSidebar = () => {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  /**
   * 设置侧边栏折叠状态
   */
  const setSidebarCollapsed = (collapsed: boolean) => {
    sidebarCollapsed.value = collapsed
  }

  /**
   * 点击左侧导航菜单时调用
   * - AI助手：总是创建新标签页
   * - 知识库、多问、设置：如果已存在则切换，否则创建
   */
  const navigateToModule = (module: NavModule) => {
    activeModule.value = module

    // 检查是否为单标签模块
    if (singleTabModules.includes(module)) {
      // 查找该模块是否已有标签页
      const existingTab = tabs.value.find((tab) => tab.module === module)
      if (existingTab) {
        // 已存在，切换到该标签页
        activeTabId.value = existingTab.id
        return
      }
    }

    // 创建新标签页
    const id = createTabId()
    const newTab: Tab = {
      id,
      titleKey: moduleLabels[module],
      module,
      icon: DefaultTabIcon,
    }
    tabs.value.push(newTab)
    activeTabId.value = id
  }

  /**
   * 添加新标签页
   */
  const addTab = (tab: Omit<Tab, 'id'>) => {
    const id = createTabId()
    const newTab: Tab = { ...tab, icon: tab.icon ?? DefaultTabIcon, id }
    tabs.value.push(newTab)
    activeTabId.value = id
    return id
  }

  /**
   * 关闭标签页
   */
  const closeTab = (tabId: string) => {
    const index = tabs.value.findIndex((tab) => tab.id === tabId)
    if (index === -1) return

    tabs.value.splice(index, 1)

    // 如果关闭的是当前激活的标签页，切换到相邻标签
    if (activeTabId.value === tabId) {
      if (tabs.value.length > 0) {
        // 优先切换到右侧标签，否则切换到左侧
        const newIndex = Math.min(index, tabs.value.length - 1)
        activeTabId.value = tabs.value[newIndex].id
        // 同步切换到标签页对应的模块
        activeModule.value = tabs.value[newIndex].module
      } else {
        activeTabId.value = null
      }
    }
  }

  /**
   * 关闭其他标签页（保留指定标签页）
   */
  const closeOtherTabs = (tabId: string) => {
    const tab = tabs.value.find((t) => t.id === tabId)
    if (!tab) return

    tabs.value = [tab]
    activeTabId.value = tabId
    activeModule.value = tab.module
  }

  /**
   * 关闭所有标签页
   */
  const closeAllTabs = () => {
    tabs.value = []
    activeTabId.value = null
  }

  /**
   * 切换标签页
   */
  const setActiveTab = (tabId: string) => {
    const tab = tabs.value.find((t) => t.id === tabId)
    if (tab) {
      activeTabId.value = tabId
      // 同步切换到标签页对应的模块
      activeModule.value = tab.module
    }
  }

  return {
    activeModule,
    sidebarCollapsed,
    tabs,
    activeTabId,
    activeTab,
    toggleSidebar,
    setSidebarCollapsed,
    navigateToModule,
    addTab,
    closeTab,
    closeOtherTabs,
    closeAllTabs,
    setActiveTab,
  }
})
