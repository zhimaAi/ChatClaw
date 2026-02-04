<script setup lang="ts">
/**
 * AI 模型选择组件
 * 水平展示可选的 AI 模型卡片，支持多选和拖拽排序
 * 长按 1 秒后可拖拽
 */
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { ProviderIcon } from '@/components/ui/provider-icon'
import SelectedIcon from '@/assets/icons/selected.svg'

const { t } = useI18n()

interface AIModel {
  id: string
  name: string
  icon: string
  displayName?: string
}

interface Props {
  models: AIModel[]
  selectedIds: string[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  toggle: [modelId: string]
  reorder: [models: AIModel[]]
}>()

// 长按状态
const LONG_PRESS_DURATION = 1000 // 长按 1 秒
const longPressTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const isDraggable = ref(false) // 是否处于可拖拽状态
const pressedIndex = ref<number | null>(null)

// 拖拽状态
const isDragging = ref(false)
const draggedIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const hasDragged = ref(false)

// 鼠标位置（用于幽灵元素跟随）
const mouseX = ref(0)
const mouseY = ref(0)

// 容器引用
const containerRef = ref<HTMLElement | null>(null)

// 悬浮提示状态
const hoveredIndex = ref<number | null>(null)
const hoverTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const tooltipPosition = ref({ x: 0, y: 0 })
const HOVER_DELAY = 300 // 悬浮 300ms 后显示提示

const handleMouseEnter = (event: MouseEvent, index: number) => {
  // 拖拽中不显示提示
  if (isDragging.value) return
  
  // 获取按钮位置用于定位 tooltip
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()
  tooltipPosition.value = {
    x: rect.left + rect.width / 2,
    y: rect.bottom + 4
  }
  
  hoverTimer.value = setTimeout(() => {
    hoveredIndex.value = index
  }, HOVER_DELAY)
}

const handleMouseLeaveItem = () => {
  if (hoverTimer.value) {
    clearTimeout(hoverTimer.value)
    hoverTimer.value = null
  }
  hoveredIndex.value = null
}

/**
 * 检查模型是否被选中
 */
const isSelected = (modelId: string) => {
  return props.selectedIds.includes(modelId)
}

/**
 * 被拖拽的模型
 */
const draggedModel = computed(() => {
  if (draggedIndex.value === null) return null
  return props.models[draggedIndex.value]
})

/**
 * 处理鼠标按下 - 开始长按计时
 */
const handleMouseDown = (event: MouseEvent, index: number) => {
  // 防止文本选择
  event.preventDefault()
  
  pressedIndex.value = index
  hasDragged.value = false
  mouseX.value = event.clientX
  mouseY.value = event.clientY
  
  // 开始长按计时
  longPressTimer.value = setTimeout(() => {
    isDraggable.value = true
  }, LONG_PRESS_DURATION)
}

/**
 * 处理鼠标移动 - 自定义拖拽
 */
const handleMouseMove = (event: MouseEvent) => {
  if (!isDraggable.value || pressedIndex.value === null) return
  
  // 更新鼠标位置
  mouseX.value = event.clientX
  mouseY.value = event.clientY
  
  // 开始拖拽
  if (!isDragging.value) {
    isDragging.value = true
    draggedIndex.value = pressedIndex.value
  }
  
  // 检测鼠标位置对应的目标元素
  if (!containerRef.value) return
  
  const buttons = containerRef.value.querySelectorAll('button')
  let targetIndex: number | null = null
  
  buttons.forEach((btn, index) => {
    const rect = btn.getBoundingClientRect()
    if (
      event.clientX >= rect.left &&
      event.clientX <= rect.right &&
      event.clientY >= rect.top &&
      event.clientY <= rect.bottom
    ) {
      targetIndex = index
    }
  })
  
  if (targetIndex !== null && targetIndex !== draggedIndex.value) {
    hasDragged.value = true
    dragOverIndex.value = targetIndex
  } else if (targetIndex === draggedIndex.value) {
    dragOverIndex.value = null
  }
}

/**
 * 处理鼠标抬起 - 完成拖拽或点击
 */
const handleMouseUp = (modelId: string) => {
  // 如果发生了拖拽，执行排序
  if (isDragging.value && dragOverIndex.value !== null && draggedIndex.value !== null) {
    const newModels = [...props.models]
    const [draggedItem] = newModels.splice(draggedIndex.value, 1)
    newModels.splice(dragOverIndex.value, 0, draggedItem)
    emit('reorder', newModels)
  } else if (!hasDragged.value && !isDragging.value) {
    // 如果未拖拽，触发点击
    emit('toggle', modelId)
  }
  
  resetState()
}

/**
 * 全局鼠标抬起处理
 */
const handleGlobalMouseUp = () => {
  if (isDragging.value && dragOverIndex.value !== null && draggedIndex.value !== null) {
    const newModels = [...props.models]
    const [draggedItem] = newModels.splice(draggedIndex.value, 1)
    newModels.splice(dragOverIndex.value, 0, draggedItem)
    emit('reorder', newModels)
  }
  resetState()
}

/**
 * 处理鼠标离开按钮
 */
const handleMouseLeave = () => {
  if (!isDragging.value) {
    cancelLongPress()
  }
}

/**
 * 取消长按计时
 */
const cancelLongPress = () => {
  if (longPressTimer.value) {
    clearTimeout(longPressTimer.value)
    longPressTimer.value = null
  }
  if (!isDragging.value) {
    isDraggable.value = false
    pressedIndex.value = null
  }
}

/**
 * 重置所有状态
 */
const resetState = () => {
  cancelLongPress()
  isDragging.value = false
  isDraggable.value = false
  draggedIndex.value = null
  dragOverIndex.value = null
  pressedIndex.value = null
  hasDragged.value = false
}

// 全局事件监听
onMounted(() => {
  document.addEventListener('mousemove', handleMouseMove)
  document.addEventListener('mouseup', handleGlobalMouseUp)
})

onUnmounted(() => {
  document.removeEventListener('mousemove', handleMouseMove)
  document.removeEventListener('mouseup', handleGlobalMouseUp)
  if (longPressTimer.value) {
    clearTimeout(longPressTimer.value)
  }
  if (hoverTimer.value) {
    clearTimeout(hoverTimer.value)
  }
})
</script>

<template>
  <div
    ref="containerRef"
    :class="cn(
      'relative -mx-2 flex items-center gap-2 overflow-x-auto px-2 py-2 select-none',
      isDragging && 'cursor-grabbing'
    )"
  >
    <button
      v-for="(model, index) in models"
      :key="model.id"
      type="button"
      :class="cn(
        'relative flex h-[62px] w-[54px] shrink-0 flex-col items-center gap-1 rounded-md p-1 transition-all',
        isSelected(model.id)
          ? 'bg-[#f5f5f5]'
          : 'bg-background hover:bg-muted/50',
        // 可拖拽状态 - 抓取手型
        isDraggable && pressedIndex === index && !isDragging && 'cursor-grab ring-2 ring-primary/50',
        // 拖拽中
        isDragging && 'cursor-grabbing',
        // 被拖拽项 - 半透明占位
        draggedIndex === index && 'opacity-30',
        // 拖拽目标 - 高亮
        dragOverIndex === index && draggedIndex !== index && 'scale-110 ring-2 ring-primary',
        // 默认状态
        !isDraggable && !isDragging && 'cursor-pointer'
      )"
      @mouseenter="handleMouseEnter($event, index)"
      @mouseleave="handleMouseLeaveItem(); handleMouseLeave()"
      @mousedown="handleMouseDown($event, index)"
      @mouseup="handleMouseUp(model.id)"
    >
      <!-- 选中标记（左上角勾选图标） -->
      <SelectedIcon
        v-if="isSelected(model.id)"
        class="absolute left-0 top-0 size-6 rounded-tl-md"
      />

      <!-- 模型图标 -->
      <div class="flex size-8 items-center justify-center rounded-md border border-border bg-background">
        <ProviderIcon :icon="model.icon" :size="24" />
      </div>

      <!-- 模型名称 -->
      <span class="w-full truncate text-center text-xs text-muted-foreground">
        {{ model.displayName || model.name }}
      </span>
    </button>

    <!-- 拖拽幽灵元素 - 跟随鼠标 -->
    <Teleport to="body">
      <div
        v-if="isDragging && draggedModel"
        class="pointer-events-none fixed z-[9999] flex h-[62px] w-[54px] flex-col items-center gap-1 rounded-md border-2 border-primary bg-background p-1 shadow-lg"
        :style="{
          left: `${mouseX - 27}px`,
          top: `${mouseY - 31}px`,
        }"
      >
        <!-- 选中标记 -->
        <SelectedIcon
          v-if="isSelected(draggedModel.id)"
          class="absolute left-0 top-0 size-6 rounded-tl-md"
        />

        <!-- 模型图标 -->
        <div class="flex size-8 items-center justify-center rounded-md border border-border bg-background">
          <ProviderIcon :icon="draggedModel.icon" :size="24" />
        </div>

        <!-- 模型名称 -->
        <span class="w-full truncate text-center text-xs text-muted-foreground">
          {{ draggedModel.displayName || draggedModel.name }}
        </span>
      </div>
    </Teleport>

    <!-- 悬浮提示 - 使用 Teleport 避免被 overflow 裁剪 -->
    <Teleport to="body">
      <Transition name="tooltip">
        <div
          v-if="hoveredIndex !== null && !isDragging && !isDraggable"
          class="pointer-events-none fixed z-[9998] whitespace-nowrap rounded-md bg-gray-900 px-2.5 py-1.5 text-xs text-white shadow-lg"
          :style="{
            left: `${tooltipPosition.x}px`,
            top: `${tooltipPosition.y}px`,
            transform: 'translateX(-50%)'
          }"
        >
          {{ t('multiask.longPressToDrag') }}
          <!-- 小三角箭头 -->
          <div class="absolute -top-1 left-1/2 -translate-x-1/2 border-4 border-transparent border-b-gray-900" />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.cursor-grabbing,
.cursor-grabbing * {
  cursor: grabbing !important;
}

/* 提示框过渡动画 */
.tooltip-enter-active,
.tooltip-leave-active {
  transition: opacity 0.15s ease;
}

.tooltip-enter-from,
.tooltip-leave-to {
  opacity: 0;
}
</style>
