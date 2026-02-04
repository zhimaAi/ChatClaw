<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'
import { MainLayout } from '@/components/layout'
import { Toaster } from '@/components/ui/toast'
import { useNavigationStore, type NavModule } from '@/stores'
import SettingsPage from '@/pages/settings/SettingsPage.vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import KnowledgePage from '@/pages/knowledge/KnowledgePage.vue'

const navigationStore = useNavigationStore()

/**
 * 模块到组件的映射
 */
const moduleComponents: Record<NavModule, unknown> = {
  assistant: AssistantPage,
  knowledge: KnowledgePage,
  settings: SettingsPage,
  multiask: null, // TODO: 实现多问页面
}

/**
 * 默认至少保持 1 个标签页：
 * - 启动时若没有标签页，自动打开一个 AI助手
 * - 当用户关闭到 0 个标签页时，自动再打开一个 AI助手
 */
watch(
  () => navigationStore.tabs.length,
  (len) => {
    if (len === 0) {
      navigationStore.navigateToModule('assistant')
    }
  },
  { immediate: true }
)

/**
 * 主题变化监听 - 当主题切换时更新所有 assistant 标签页的默认图标
 * 精确检测 dark class 变化，避免其他 class 变化触发不必要的刷新
 */
let themeObserver: InstanceType<typeof window.MutationObserver> | null = null
let wasDarkMode = document.documentElement.classList.contains('dark')

onMounted(() => {
  themeObserver = new window.MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.attributeName === 'class') {
        const isDarkMode = document.documentElement.classList.contains('dark')
        // 只在 dark 模式实际切换时才刷新图标
        if (wasDarkMode !== isDarkMode) {
          wasDarkMode = isDarkMode
          navigationStore.refreshAssistantDefaultIcons()
        }
      }
    }
  })
  themeObserver.observe(document.documentElement, { attributes: true })
})

onUnmounted(() => {
  themeObserver?.disconnect()
})
</script>

<template>
  <Toaster />
  <MainLayout>
    <!--
      标签页状态保留架构：
      - 为每个打开的标签页渲染独立的组件实例（通过 :key="tab.id" 确保独立）
      - 使用 v-show 控制显示/隐藏，而不是 v-if 销毁组件
      - 这样切换标签页时，组件实例不会被销毁，所有状态自然保留
    -->
    <template v-for="tab in navigationStore.tabs" :key="tab.id">
      <component
        :is="moduleComponents[tab.module]"
        v-if="moduleComponents[tab.module]"
        v-show="navigationStore.activeTabId === tab.id"
        :tab-id="tab.id"
        class="h-full w-full"
      />
    </template>
  </MainLayout>
</template>
