<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { MainLayout } from '@/components/layout'
import { Toaster } from '@/components/ui/toast'
import { useNavigationStore, type NavModule } from '@/stores'
import SettingsPage from '@/pages/settings/SettingsPage.vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import KnowledgePage from '@/pages/knowledge/KnowledgePage.vue'
import { Events, System } from '@wailsio/runtime'
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { TextSelectionService } from '@bindings/willchat/internal/services/textselection'
import MultiaskPage from '@/pages/multiask/MultiaskPage.vue'

const navigationStore = useNavigationStore()
const activeTab = computed(() => navigationStore.activeTab)

/**
 * 模块到组件的映射
 */
const moduleComponents: Record<NavModule, unknown> = {
  assistant: AssistantPage,
  knowledge: KnowledgePage,
  settings: SettingsPage,
  multiask: MultiaskPage,
}

/**
 * 是否显示一问多答页面
 */
const showMultiask = computed(() => activeTab.value?.module === 'multiask')

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
 * 处理划词弹窗按钮点击事件
 * 根据吸附窗体状态决定发送文本到哪里
 */
let unsubscribeTextSelection: (() => void) | null = null
let onMouseUp: ((e: MouseEvent) => void) | null = null

/**
 * 主题变化监听 - 当主题切换时更新所有 assistant 标签页的默认图标
 * 精确检测 dark class 变化，避免其他 class 变化触发不必要的刷新
 */
let themeObserver: InstanceType<typeof window.MutationObserver> | null = null
let wasDarkMode = document.documentElement.classList.contains('dark')

onMounted(() => {
  // Text selection event handling
  unsubscribeTextSelection = Events.On('text-selection:action', async (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
    const text = payload?.text ?? ''
    if (!text) return

    try {
      // Check snap window state
      const status = await SnapService.GetStatus()

      if (status.state === 'attached') {
        // Snap window is attached, send text to snap window
        // Also wake winsnap + target to the front so the user can see it.
        void SnapService.WakeAttached()
        Events.Emit('text-selection:send-to-snap', { text })
      } else {
        // Snap window is not attached (stopped or hidden)
        // Navigate to AI assistant and send text there
        if (activeTab.value?.module !== 'assistant') {
          navigationStore.navigateToModule('assistant')
        }
        // Emit event for assistant page to receive text
        Events.Emit('text-selection:send-to-assistant', { text })
      }
    } catch (error) {
      console.error('Failed to get snap status:', error)
      // Fallback: send to assistant
      if (activeTab.value?.module !== 'assistant') {
        navigationStore.navigateToModule('assistant')
      }
      Events.Emit('text-selection:send-to-assistant', { text })
    }
  })

  // In-app text selection: global mouseup listener (mouse hook skips our own windows).
  onMouseUp = (e: MouseEvent) => {
    // Only react to left button.
    if (e.button !== 0) return
    const sel = window.getSelection?.()
    const text = sel?.toString?.().trim?.() ?? ''
    if (!text) return
    // Best-effort: use screen coordinates so popup works for both main & other windows.
    // macOS: backend mouse hook uses physical pixels; browser events are in CSS pixels (points).
    const scale = System.IsMac() ? window.devicePixelRatio || 1 : 1
    void TextSelectionService.ShowAtScreenPos(text, Math.round(e.screenX * scale), Math.round(e.screenY * scale))
  }
  window.addEventListener('mouseup', onMouseUp, true)

  // Theme change observer
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
  unsubscribeTextSelection?.()
  unsubscribeTextSelection = null
  if (onMouseUp) {
    window.removeEventListener('mouseup', onMouseUp, true)
    onMouseUp = null
  }
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
