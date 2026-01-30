<script setup lang="ts">
/**
 * 标题栏组件
 * 包含侧边栏折叠按钮和多标签页
 */
import { computed } from 'vue'
import { System } from '@wailsio/runtime'
import { useI18n } from 'vue-i18n'
import { useNavigationStore } from '@/stores'
import { cn } from '@/lib/utils'
import IconSidebarToggle from '@/assets/icons/sidebar-toggle.svg'
import IconAddNewTab from '@/assets/icons/add-new-tab.svg'

const navigationStore = useNavigationStore()
const { t } = useI18n()

const isMac = computed(() => System.IsMac())

/**
 * 处理标签页点击
 */
const handleTabClick = (tabId: string) => {
  navigationStore.setActiveTab(tabId)
}

/**
 * 处理标签页关闭
 */
const handleTabClose = (event: { stopPropagation?: () => void } | null, tabId: string) => {
  event?.stopPropagation?.()
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
  navigationStore.navigateToModule('assistant', t)
}
</script>

<template>
  <div
    class="flex h-8 items-end gap-2 overflow-hidden bg-[#dee8fa] pr-2"
    style="--wails-draggable: drag"
  >
    <!-- 左侧：macOS 窗口控制按钮区域 + 侧边栏折叠按钮（单独居中对齐，避免受 tabs 的贴底布局影响） -->
    <div
      :class="
        cn(
          'flex h-full shrink-0 items-start gap-2 pt-0.5',
          // macOS 下视觉基线略靠下，这里把左侧控制区整体下移一点
          isMac && 'translate-y-[3px]'
        )
      "
    >
      <!-- macOS窗口控制按钮占位区域（Wails会自动渲染在这个位置） -->
      <!-- macOS需要约70px，Windows不需要占位 -->
      <div :class="cn('shrink-0', isMac ? 'w-[70px]' : 'w-2')" />

      <!-- 侧边栏展开/收起按钮 -->
      <button
        class="flex size-6 shrink-0 items-center justify-center rounded hover:bg-[#ccddf5]"
        style="--wails-draggable: no-drag"
        @click="handleToggleSidebar"
      >
        <img :src="IconSidebarToggle" alt="" class="size-4" />
      </button>
    </div>

    <!-- 标签页列表 -->
    <div class="z-2 flex shrink-0 items-end gap-1 self-stretch">
      <button
        v-for="tab in navigationStore.tabs"
        :key="tab.id"
        :class="
          cn(
            'group relative flex h-8 items-center justify-between gap-2 rounded-t-xl px-4',
            'transition-colors duration-150',
            navigationStore.activeTabId === tab.id
              ? 'bg-background text-foreground'
              : 'bg-[#dee8fa] text-muted-foreground hover:bg-[#ccddf5]'
          )
        "
        style="--wails-draggable: no-drag"
        @click="handleTabClick(tab.id)"
      >
        <div class="flex items-center gap-2">
          <!-- 标签页图标 -->
          <div
            v-if="tab.icon"
            class="size-5 shrink-0 overflow-hidden rounded-md"
          >
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
          <span class="max-w-[100px] truncate text-sm">{{ tab.title }}</span>
        </div>
        <!-- 关闭按钮 -->
        <div
          class="flex size-4 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
          @click="handleTabClose($event, tab.id)"
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

      <!-- + 按钮应紧挨最后一个标签页 -->
      <button
        class="flex size-8 shrink-0 items-center justify-center rounded hover:bg-[#ccddf5]"
        style="--wails-draggable: no-drag"
        @click="handleAddAssistantTab"
      >
        <img :src="IconAddNewTab" alt="" class="size-4" />
      </button>
    </div>
  </div>
</template>
