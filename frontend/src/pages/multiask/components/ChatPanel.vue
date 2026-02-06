<script setup lang="ts">
/**
 * 聊天面板组件
 * 通过后端 WebView 服务嵌入 AI 网站界面
 * 面板仅显示标题栏，WebView 内容由后端管理
 */
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface AIModel {
  id: string
  name: string
  icon: string
  displayName?: string
  url?: string
}

interface Props {
  model: AIModel
  /** 面板 ID（用于与后端 WebView 关联） */
  panelId: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  /** 面板容器挂载完成，返回位置信息 */
  mounted: [bounds: { x: number; y: number; width: number; height: number }]
  /** 面板容器尺寸变化 */
  resize: [bounds: { x: number; y: number; width: number; height: number }]
}>()

const containerRef = ref<HTMLElement>()

/**
 * 获取面板容器的位置和大小
 * 返回相对于窗口客户区域的坐标
 */
const getBounds = () => {
  if (!containerRef.value) return null
  const rect = containerRef.value.getBoundingClientRect()

  // 在 Wails 中，主 WebView 占据整个窗口客户区域
  // getBoundingClientRect 返回的坐标就是相对于窗口客户区域的
  const bounds = {
    x: Math.round(rect.left),
    y: Math.round(rect.top),
    width: Math.round(rect.width),
    height: Math.round(rect.height),
  }

  console.log(`[ChatPanel] getBounds for ${props.panelId}:`, bounds)
  return bounds
}

/**
 * 通知父组件位置信息
 */
const notifyBounds = () => {
  const bounds = getBounds()
  if (bounds && bounds.width > 0 && bounds.height > 0) {
    emit('resize', bounds)
  }
}

// 监听窗口大小变化
let resizeObserver: ResizeObserver | null = null

onMounted(async () => {
  // 等待 DOM 完全渲染
  await nextTick()

  // 延迟一小段时间确保布局稳定
  setTimeout(() => {
    const bounds = getBounds()
    if (bounds && bounds.width > 0 && bounds.height > 0) {
      console.log(`[ChatPanel] Mounted, emitting bounds for ${props.panelId}:`, bounds)
      emit('mounted', bounds)
    } else {
      console.warn(`[ChatPanel] Invalid bounds on mount for ${props.panelId}:`, bounds)
    }
  }, 100)

  // 监听容器大小变化
  if (containerRef.value) {
    resizeObserver = new ResizeObserver(() => {
      notifyBounds()
    })
    resizeObserver.observe(containerRef.value)
  }

  // 监听窗口大小变化
  window.addEventListener('resize', notifyBounds)
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  window.removeEventListener('resize', notifyBounds)
})

// 暴露方法供父组件调用
defineExpose({
  getBounds,
  notifyBounds,
})
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden rounded-md border border-border bg-background">
    <!-- 面板标题栏（无底部边框，与 WebView 一体化） -->
    <div class="flex h-10 shrink-0 items-center justify-center px-4">
      <span class="truncate text-sm font-medium text-foreground">
        {{ model.displayName || model.name }}
      </span>
    </div>

    <!-- WebView 容器区域（由后端渲染 WebView） -->
    <div ref="containerRef" class="webview-box relative flex-1" :data-panel-id="panelId">
      <!-- 这个区域会被后端的 WebView 覆盖 -->
      <!-- 在 WebView 加载前显示占位内容 -->
      <div class="absolute inset-0 flex items-center justify-center bg-muted/20">
        <div class="flex flex-col items-center gap-2 text-muted-foreground">
          <div class="size-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
          <span class="text-xs">{{ t('multiask.loading') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.webview-box {
  margin-bottom: 4px;
}
</style>
