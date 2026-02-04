<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MainLayout } from '@/components/layout'
import { Toaster } from '@/components/ui/toast'
import { useNavigationStore } from '@/stores'
import SettingsPage from '@/pages/settings/SettingsPage.vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import KnowledgePage from '@/pages/knowledge/KnowledgePage.vue'
import { Events, System } from '@wailsio/runtime'
import { SnapService } from '@bindings/willchat/internal/services/windows'
import { TextSelectionService } from '@bindings/willchat/internal/services/textselection'
import MultiaskPage from '@/pages/multiask/MultiaskPage.vue'

const { t } = useI18n()
const navigationStore = useNavigationStore()

/**
 * 当前激活的标签页
 */
const activeTab = computed(() => navigationStore.activeTab)

/**
 * 是否显示设置页面
 */
const showSettings = computed(() => activeTab.value?.module === 'settings')
const showAssistant = computed(() => activeTab.value?.module === 'assistant')
const showKnowledge = computed(() => activeTab.value?.module === 'knowledge')

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
    <!-- 设置页面 -->
    <SettingsPage v-if="showSettings" />
    <!-- 知识库页面 -->
    <KnowledgePage v-else-if="showKnowledge" />

    <!-- AI助手页面 -->
    <AssistantPage v-else-if="showAssistant" />

    <!-- 一问多答页面 -->
    <MultiaskPage v-else-if="showMultiask" />

    <!-- 主内容区域 - 显示当前模块的占位内容 -->
    <div v-else class="flex h-full w-full items-center justify-center bg-background">
      <div class="flex flex-col items-center gap-4">
        <!-- 显示当前标签页标题，如果没有标签页则显示提示 -->
        <template v-if="activeTab">
          <h1 class="text-2xl font-semibold text-foreground">
            {{ activeTab.titleKey ? t(activeTab.titleKey) : activeTab.title }}
          </h1>
          <p class="text-muted-foreground">
            {{ t('app.title') }}
          </p>
        </template>
        <template v-else>
          <h1 class="text-2xl font-semibold text-foreground">
            {{ t('app.title') }}
          </h1>
          <p class="text-muted-foreground">
            {{ t('nav.assistant') }}
          </p>
        </template>
      </div>
    </div>
  </MainLayout>
</template>
