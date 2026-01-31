<script setup lang="ts">
/**
 * 窗口控制按钮组件
 * macOS: 显示在左上角（红/黄/绿三色按钮）
 *   - 红色：关闭
 *   - 黄色：最小化
 *   - 绿色：全屏（不是最大化）
 * Windows: 显示在右上角（最小化/最大化/关闭按钮）
 */
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { System, Window, Events } from '@wailsio/runtime'
import { cn } from '@/lib/utils'

const props = defineProps<{
  /** 按钮位置：left 用于 macOS，right 用于 Windows */
  position?: 'left' | 'right'
}>()

const isMac = computed(() => System.IsMac())
const isMaximised = ref(false)
const isFullscreen = ref(false)

// 根据平台自动确定位置
const buttonPosition = computed(() => {
  if (props.position) return props.position
  return isMac.value ? 'left' : 'right'
})

/**
 * 更新窗口状态
 */
const updateWindowState = async () => {
  isMaximised.value = await Window.IsMaximised()
  isFullscreen.value = await Window.IsFullscreen()
}

/**
 * 最小化窗口
 */
const handleMinimise = () => {
  Window.Minimise()
}

/**
 * 切换最大化/还原窗口（Windows 用）
 */
const handleMaximise = async () => {
  if (isMaximised.value) {
    await Window.UnMaximise()
  } else {
    await Window.Maximise()
  }
  await updateWindowState()
}

/**
 * 切换全屏/退出全屏（macOS 绿色按钮用）
 */
const handleFullscreen = async () => {
  if (isFullscreen.value) {
    await Window.UnFullscreen()
  } else {
    await Window.Fullscreen()
  }
  await updateWindowState()
}

/**
 * 关闭窗口
 */
const handleClose = () => {
  Window.Close()
}

// 监听窗口状态变化
let unsubscribeResize: (() => void) | null = null
let unsubscribeFullscreen: (() => void) | null = null
let unsubscribeUnFullscreen: (() => void) | null = null

onMounted(async () => {
  await updateWindowState()

  // 监听窗口事件以更新状态
  unsubscribeResize = Events.On('window:resize', updateWindowState)
  unsubscribeFullscreen = Events.On('window:fullscreen', () => {
    isFullscreen.value = true
  })
  unsubscribeUnFullscreen = Events.On('window:unfullscreen', () => {
    isFullscreen.value = false
  })
})

onUnmounted(() => {
  unsubscribeResize?.()
  unsubscribeFullscreen?.()
  unsubscribeUnFullscreen?.()
})
</script>

<template>
  <!-- macOS 风格按钮（左侧） -->
  <div
    v-if="isMac && buttonPosition === 'left'"
    class="group flex items-center gap-2"
    style="--wails-draggable: no-drag"
  >
    <!-- 关闭按钮（红色） -->
    <button
      :class="
        cn(
          'flex size-3 items-center justify-center rounded-full',
          'bg-[#ff5f57] hover:bg-[#ff5f57]/80',
          'transition-colors duration-150'
        )
      "
      @click="handleClose"
    >
      <svg
        class="size-2 opacity-0 group-hover:opacity-100"
        viewBox="0 0 12 12"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="M3 3l6 6M9 3l-6 6"
          stroke="rgba(0,0,0,0.5)"
          stroke-width="1.5"
          stroke-linecap="round"
        />
      </svg>
    </button>

    <!-- 最小化按钮（黄色） -->
    <button
      :class="
        cn(
          'flex size-3 items-center justify-center rounded-full',
          'bg-[#febc2e] hover:bg-[#febc2e]/80',
          'transition-colors duration-150'
        )
      "
      @click="handleMinimise"
    >
      <svg
        class="size-2 opacity-0 group-hover:opacity-100"
        viewBox="0 0 12 12"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path d="M2.5 6h7" stroke="rgba(0,0,0,0.5)" stroke-width="1.5" stroke-linecap="round" />
      </svg>
    </button>

    <!-- 全屏/退出全屏按钮（绿色） -->
    <button
      :class="
        cn(
          'flex size-3 items-center justify-center rounded-full',
          'bg-[#28c840] hover:bg-[#28c840]/80',
          'transition-colors duration-150'
        )
      "
      @click="handleFullscreen"
    >
      <svg
        v-if="isFullscreen"
        class="size-2 opacity-0 group-hover:opacity-100"
        viewBox="0 0 12 12"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <!-- 退出全屏图标：向内的箭头 -->
        <path
          d="M4 2v2H2M10 4H8V2M8 10V8h2M2 8h2v2"
          stroke="rgba(0,0,0,0.5)"
          stroke-width="1"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
      <svg
        v-else
        class="size-2 opacity-0 group-hover:opacity-100"
        viewBox="0 0 12 12"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <!-- 全屏图标：向外的箭头 -->
        <path
          d="M2 4V2h2M10 2v2h-2M8 10h2V8M2 8v2h2"
          stroke="rgba(0,0,0,0.5)"
          stroke-width="1"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </button>
  </div>

  <!-- Windows 风格按钮（右侧） -->
  <div
    v-else-if="!isMac && buttonPosition === 'right'"
    class="flex h-full items-stretch"
    style="--wails-draggable: no-drag"
  >
    <!-- 最小化按钮 -->
    <button
      :class="
        cn(
          'flex w-11 items-center justify-center',
          'text-foreground/70 hover:bg-foreground/10 hover:text-foreground',
          'transition-colors duration-150'
        )
      "
      @click="handleMinimise"
    >
      <svg class="size-4" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M4 8h8" stroke="currentColor" stroke-width="1" stroke-linecap="round" />
      </svg>
    </button>

    <!-- 最大化/还原按钮 -->
    <button
      :class="
        cn(
          'flex w-11 items-center justify-center',
          'text-foreground/70 hover:bg-foreground/10 hover:text-foreground',
          'transition-colors duration-150'
        )
      "
      @click="handleMaximise"
    >
      <svg
        v-if="isMaximised"
        class="size-4"
        viewBox="0 0 16 16"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <!-- 还原图标 -->
        <rect x="5" y="5" width="7" height="7" stroke="currentColor" stroke-width="1" fill="none" />
        <path d="M6 5V4H13V11H12" stroke="currentColor" stroke-width="1" fill="none" />
      </svg>
      <svg v-else class="size-4" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <!-- 最大化图标 -->
        <rect
          x="3"
          y="3"
          width="10"
          height="10"
          stroke="currentColor"
          stroke-width="1"
          fill="none"
        />
      </svg>
    </button>

    <!-- 关闭按钮 -->
    <button
      :class="
        cn(
          'flex w-11 items-center justify-center',
          'text-foreground/70 hover:bg-[#e81123] hover:text-white',
          'transition-colors duration-150'
        )
      "
      @click="handleClose"
    >
      <svg class="size-4" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path
          d="M4 4l8 8M12 4l-8 8"
          stroke="currentColor"
          stroke-width="1"
          stroke-linecap="round"
        />
      </svg>
    </button>
  </div>
</template>
