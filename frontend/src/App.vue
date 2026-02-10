<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { MainLayout } from '@/components/layout'
import { Toaster } from '@/components/ui/toast'
import { useNavigationStore, useAppStore, type NavModule } from '@/stores'
import SettingsPage from '@/pages/settings/SettingsPage.vue'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import KnowledgePage from '@/pages/knowledge/KnowledgePage.vue'
import { Events, System, Window } from '@wailsio/runtime'
import { TextSelectionService } from '@bindings/willclaw/internal/services/textselection'
import { UpdaterService } from '@bindings/willclaw/internal/services/updater'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'
import MultiaskPage from '@/pages/multiask/MultiaskPage.vue'
import { SnapService } from '@bindings/willclaw/internal/services/windows'
import UpdateDialog from '@/pages/settings/components/UpdateDialog.vue'
const navigationStore = useNavigationStore()
const appStore = useAppStore()
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
// --- Global update dialog state ---
const updateDialogOpen = ref(false)
const updateDialogMode = ref<'new-version' | 'just-updated'>('new-version')
const updateDialogVersion = ref('')
const updateDialogNotes = ref('')

let unsubscribeUpdateAvailable: (() => void) | null = null
let unsubscribeShowDialog: (() => void) | null = null

let unsubscribeTextSelection: (() => void) | null = null
let onMouseUp: ((e: MouseEvent) => void) | null = null
let onKeyDownCapture: ((e: KeyboardEvent) => void) | null = null
let onKeyDownMacMinimize: ((e: KeyboardEvent) => void) | null = null

/**
 * 主题变化监听 - 当主题切换时更新所有 assistant 标签页的默认图标
 * 精确检测 dark class 变化，避免其他 class 变化触发不必要的刷新
 */
let themeObserver: InstanceType<typeof window.MutationObserver> | null = null
let wasDarkMode = document.documentElement.classList.contains('dark')

onMounted(async () => {
  // --- Global update monitoring ---

  // Check for pending update on startup (just-updated scenario)
  try {
    const pending = await UpdaterService.GetPendingUpdate()
    if (pending && pending.latest_version) {
      updateDialogVersion.value = pending.latest_version
      updateDialogNotes.value = pending.release_notes || ''
      updateDialogMode.value = 'just-updated'
      updateDialogOpen.value = true
    }
  } catch {
    // Ignore — no pending update
  }

  // Listen for backend auto-check event (ServiceStartup emits after 3s).
  // Always mark the badge so the user sees a dot on "Check for Update".
  // Only auto-open the dialog when auto_update is enabled.
  unsubscribeUpdateAvailable = Events.On('update:available', async (event: any) => {
    const info = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (info?.has_update && info?.latest_version) {
      // Always mark update available (for badge display)
      appStore.hasAvailableUpdate = true

      // Only auto-show the update dialog when the user opted in to auto-update
      let autoUpdate = true
      try {
        const setting = await SettingsService.Get('auto_update')
        if (setting) {
          autoUpdate = setting.value !== 'false'
        }
      } catch {
        // Default to showing the dialog if we can't read the setting
      }

      if (autoUpdate) {
        updateDialogVersion.value = info.latest_version
        updateDialogNotes.value = info.release_notes || ''
        updateDialogMode.value = 'new-version'
        updateDialogOpen.value = true
      }
    }
  })

  // Listen for AboutSettings.vue manual check result
  unsubscribeShowDialog = Events.On('update:show-dialog', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    updateDialogVersion.value = payload?.version || ''
    updateDialogNotes.value = payload?.release_notes || ''
    updateDialogMode.value = payload?.mode || 'new-version'
    updateDialogOpen.value = true
  })

  // Text selection event handling
  unsubscribeTextSelection = Events.On('text-selection:action', async (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
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
    void TextSelectionService.ShowAtScreenPos(
      text,
      Math.round(e.screenX * scale),
      Math.round(e.screenY * scale)
    )
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

  // macOS Cmd+M workaround:
  // Wails v3 frameless windows lack NSWindowStyleMaskMiniaturizable, so the standard
  // Cmd+M minimize shortcut does nothing. We intercept it here and call Window.Hide()
  // (same as the yellow traffic-light button) to simulate minimize-to-dock behavior.
  if (System.IsMac()) {
    onKeyDownMacMinimize = (e: KeyboardEvent) => {
      if (e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && e.key === 'm') {
        e.preventDefault()
        void Window.Hide()
      }
    }
    window.addEventListener('keydown', onKeyDownMacMinimize, true)
  }

  // Global IME guard:
  // When the user presses Enter while composing (IME), prevent any keydown.enter handlers
  // from treating it as a "submit/send" action. We do NOT preventDefault so IME can commit text.
  onKeyDownCapture = (e: KeyboardEvent) => {
    if (e.key !== 'Enter') return

    const anyEvent = e as any
    const isComposing = !!anyEvent?.isComposing || anyEvent?.keyCode === 229
    if (!isComposing) return

    const target = e.target as HTMLElement | null
    if (!target) return

    const tag = target.tagName?.toLowerCase?.() || ''
    const isEditable =
      tag === 'textarea' ||
      tag === 'input' ||
      (target as any).isContentEditable === true ||
      target.getAttribute?.('contenteditable') === 'true'

    if (!isEditable) return

    // Block Vue key modifiers and any other listeners from seeing this Enter.
    e.stopImmediatePropagation()
  }
  window.addEventListener('keydown', onKeyDownCapture, true)
})

onUnmounted(() => {
  unsubscribeUpdateAvailable?.()
  unsubscribeUpdateAvailable = null
  unsubscribeShowDialog?.()
  unsubscribeShowDialog = null

  if (onMouseUp) {
    window.removeEventListener('mouseup', onMouseUp, true)
    onMouseUp = null
  }
  if (onKeyDownCapture) {
    window.removeEventListener('keydown', onKeyDownCapture, true)
    onKeyDownCapture = null
  }
  if (onKeyDownMacMinimize) {
    window.removeEventListener('keydown', onKeyDownMacMinimize, true)
    onKeyDownMacMinimize = null
  }
  themeObserver?.disconnect()
})
</script>

<template>
  <Toaster />

  <!-- Global update dialog (new version / just updated) -->
  <UpdateDialog
    :open="updateDialogOpen"
    :mode="updateDialogMode"
    :version="updateDialogVersion"
    :release-notes="updateDialogNotes"
    @update:open="updateDialogOpen = $event"
  />
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
