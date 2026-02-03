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
import { useNavigationStore } from '@/stores'
import { cn } from '@/lib/utils'
import IconSidebarToggle from '@/assets/icons/sidebar-toggle.svg'
import IconAddNewTab from '@/assets/icons/add-new-tab.svg'
import WindowControlButtons from './WindowControlButtons.vue'
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
 * macOS：双击标题栏区域触发窗口“缩放”（等同于绿灯按钮行为）
 */
const handleTitleBarDoubleClick = async () => {
  if (isMac.value) {
    void Window.Zoom()
  } else {
    // Windows: 切换最大化状态
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
  <div class="flex h-10 items-center overflow-hidden bg-titlebar" style="--wails-draggable: drag">
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
              <!-- 标签页图标 -->
              <div v-if="tab.icon" class="size-5 shrink-0 overflow-hidden rounded-md">
                <img :src="tab.icon" alt="" class="size-full object-cover" />
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
              <!-- 标签页标题 -->
              <span class="max-w-[100px] truncate text-sm">{{
                tab.titleKey ? t(tab.titleKey) : tab.title
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

    <!-- 右侧空白拖拽/双击区域（避免干扰 tabs 的交互） -->
    <div
      class="min-w-4 shrink-0 self-stretch"
      style="--wails-draggable: drag"
      @dblclick="handleTitleBarDoubleClick"
    />

    <!-- Windows 窗口控制按钮（右侧） -->
    <WindowControlButtons v-if="!isMac" position="right" />
  </div>
</template>
