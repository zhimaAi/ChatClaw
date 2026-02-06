<script setup lang="ts">
/**
 * 标题栏组件
 * 包含侧边栏折叠按钮、多标签页和窗口控制按钮
 * - macOS: 自定义红黄绿按钮在左侧，高度 40px
 * - Windows: 自定义窗口控制按钮在右侧，高度 40px
 */
import { computed } from 'vue'
import { System, Window } from '@wailsio/runtime'
import { useI18n } from 'vue-i18n'
import { useNavigationStore, type NavModule } from '@/stores'
import { cn } from '@/lib/utils'
import IconSidebarToggle from '@/assets/icons/sidebar-toggle.svg'
import IconAddNewTab from '@/assets/icons/add-new-tab.svg'
import IconKnowledge from '@/assets/icons/knowledge.svg'
import IconMultiask from '@/assets/icons/multiask.svg'
import IconSettings from '@/assets/icons/settings.svg'
import WindowControlButtons from './WindowControlButtons.vue'

/**
 * Module-to-icon mapping for tab icons (matching side nav icons).
 * assistant module uses dynamic image icons set by AssistantPage, so not included here.
 *
 * Note: SVG imports are typed as `string` by vite/client.d.ts but at runtime
 * vite-svg-loader (defaultImport: 'component') provides Vue components.
 * We use Record<string, any> to avoid the type conflict.
 */
const moduleTabIcons: Partial<Record<NavModule, any>> = {
  knowledge: IconKnowledge,
  multiask: IconMultiask,
  settings: IconSettings,
}
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'

const navigationStore = useNavigationStore()
const { t } = useI18n()

const isMac = computed(() => System.IsMac())

/**
 * 关闭按钮事件类型（支持点击和键盘事件）
 */
type CloseButtonEvent = MouseEvent | KeyboardEvent

/**
 * 处理标签页点击
 */
const handleTabClick = (tabId: string) => {
  navigationStore.setActiveTab(tabId)
}

/**
 * 处理标签页关闭
 */
const handleTabClose = (event: CloseButtonEvent, tabId: string) => {
  event.stopPropagation()
  navigationStore.closeTab(tabId)
}

/**
 * 切换侧边栏折叠状态
 */
const handleToggleSidebar = () => {
  navigationStore.toggleSidebar()
}

/**
 * 新建一个 AI助手 标签页（右侧 + 按钮）
 */
const handleAddAssistantTab = () => {
  navigationStore.navigateToModule('assistant')
}

/**
 * Double-click on the title bar drag area to zoom/maximize the window.
 * macOS: triggers Window.Zoom() (same as green traffic-light button behavior)
 * Windows: toggles maximize/unmaximize
 *
 * Walks up the DOM from event.target to find the nearest element with
 * an explicit --wails-draggable value:
 *   - 'no-drag' → click was on an interactive element, bail out
 *   - 'drag'    → click was on a draggable area, proceed
 */
const handleTitleBarDoubleClick = async (event: MouseEvent) => {
  let el = event.target as HTMLElement | null
  while (el) {
    const v = el.style.getPropertyValue('--wails-draggable')?.trim()
    if (v === 'no-drag') return
    if (v === 'drag') break
    el = el.parentElement
  }

  if (isMac.value) {
    void Window.Zoom()
  } else {
    // Windows: toggle maximise state
    const isMaximised = await Window.IsMaximised()
    if (isMaximised) {
      await Window.UnMaximise()
    } else {
      await Window.Maximise()
    }
  }
}
</script>

<template>
  <div
    class="flex h-10 items-center overflow-hidden bg-titlebar"
    style="--wails-draggable: drag"
    @dblclick="handleTitleBarDoubleClick"
  >
    <!-- 左侧区域 -->
    <div class="flex h-full shrink-0 items-center gap-4 pl-3">
      <!-- macOS: 自定义红黄绿按钮 -->
      <WindowControlButtons v-if="isMac" position="left" />

      <!-- 侧边栏展开/收起按钮 -->
      <button
        class="flex size-6 shrink-0 items-center justify-center rounded text-foreground/70 hover:bg-titlebar-hover hover:text-foreground"
        style="--wails-draggable: no-drag"
        @click="handleToggleSidebar"
      >
        <IconSidebarToggle class="size-4" />
      </button>
    </div>

    <!-- 标签页列表（支持横向滚动） -->
    <div
      class="z-2 ml-2 flex min-w-0 flex-1 items-center gap-1 self-stretch overflow-x-auto scrollbar-hide"
    >
      <ContextMenu v-for="tab in navigationStore.tabs" :key="tab.id">
        <ContextMenuTrigger as-child>
          <button
            :class="
              cn(
                'group relative flex h-8 select-none items-center justify-between gap-2 rounded-lg px-3',
                'transition-colors duration-150',
                navigationStore.activeTabId === tab.id
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-titlebar-hover'
              )
            "
            style="--wails-draggable: no-drag"
            @click="handleTabClick(tab.id)"
          >
            <div class="flex items-center gap-2">
              <!-- 标签页图标：优先使用模块对应的 SVG 图标，其次使用图片 URL，最后 fallback -->
              <div
                v-if="moduleTabIcons[tab.module]"
                class="flex size-5 shrink-0 items-center justify-center"
              >
                <component
                  :is="moduleTabIcons[tab.module]"
                  class="size-3.5 overflow-visible text-current"
                />
              </div>
              <div
                v-else-if="tab.icon"
                class="flex size-5 shrink-0 items-center justify-center"
              >
                <img :src="tab.icon" alt="" class="size-full object-contain" />
              </div>
              <div
                v-else
                class="flex size-5 shrink-0 items-center justify-center rounded-md bg-muted"
              >
                <svg
                  viewBox="0 0 16 16"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                  class="size-3"
                >
                  <path
                    d="M8 2a6 6 0 100 12A6 6 0 008 2z"
                    stroke="currentColor"
                    stroke-width="1.5"
                  />
                </svg>
              </div>
              <!-- 标签页标题：优先使用自定义标题，否则使用翻译键 -->
              <span class="max-w-[100px] truncate text-sm">{{
                tab.title?.trim() ? tab.title : tab.titleKey ? t(tab.titleKey) : ''
              }}</span>
            </div>
            <!-- 关闭按钮 -->
            <div
              role="button"
              tabindex="0"
              class="flex size-4 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
              @click="handleTabClose($event, tab.id)"
              @keydown.enter.prevent="handleTabClose($event, tab.id)"
              @keydown.space.prevent="handleTabClose($event, tab.id)"
            >
              <svg
                viewBox="0 0 16 16"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
                class="size-3"
              >
                <path
                  d="M4 4l8 8M12 4l-8 8"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                />
              </svg>
            </div>
          </button>
        </ContextMenuTrigger>
        <ContextMenuContent class="w-48">
          <ContextMenuItem @click="navigationStore.closeTab(tab.id)">
            {{ t('tab.close') }}
          </ContextMenuItem>
          <ContextMenuItem
            :disabled="navigationStore.tabs.length <= 1"
            @click="navigationStore.closeOtherTabs(tab.id)"
          >
            {{ t('tab.closeOthers') }}
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem @click="navigationStore.closeAllTabs()">
            {{ t('tab.closeAll') }}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>

      <!-- + 按钮应紧挨最后一个标签页，与标签页垂直居中对齐 -->
      <button
        class="flex size-7 shrink-0 items-center justify-center rounded text-foreground/70 hover:bg-titlebar-hover hover:text-foreground"
        style="--wails-draggable: no-drag"
        @click="handleAddAssistantTab"
      >
        <IconAddNewTab class="size-4" />
      </button>
    </div>

    <!-- 右侧空白拖拽区域 -->
    <div class="min-w-4 shrink-0 self-stretch" style="--wails-draggable: drag" />

    <!-- Windows 窗口控制按钮（右侧） -->
    <WindowControlButtons v-if="!isMac" position="right" />
  </div>
</template>
