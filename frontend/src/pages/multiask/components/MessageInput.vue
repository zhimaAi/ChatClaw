<script setup lang="ts">
/**
 * 消息输入框组件
 * 仅支持文本输入和发送
 */
import { computed, ref } from 'vue'

interface Props {
  placeholder?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: '请输入问题',
  disabled: false,
})

const modelValue = defineModel<string>({ default: '' })

const emit = defineEmits<{
  send: []
}>()

/**
 * 是否可以发送（输入内容不为空且未禁用）
 */
const canSend = computed(() => {
  return modelValue.value.trim().length > 0 && !props.disabled
})

/**
 * 处理发送
 */
const handleSend = () => {
  if (canSend.value) {
    emit('send')
  }
}

/**
 * 处理键盘事件
 */
const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

/**
 * Expose focus method for parent usage.
 */
const inputRef = ref<HTMLTextAreaElement | null>(null)
const focus = () => {
  inputRef.value?.focus()
}

defineExpose({ focus })
</script>

<template>
  <!-- 高度与原先一致：上方输入区域 + 下方一行图标按钮 -->
  <div
    class="relative flex min-h-[116px] w-full flex-col rounded-2xl border-2 border-border bg-background px-3 py-2.5 shadow-sm"
  >
    <!-- 输入区域 -->
    <textarea
      ref="inputRef"
      v-model="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      class="min-h-[48px] min-w-0 flex-1 resize-none bg-transparent text-base text-foreground outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
      @keydown="handleKeydown"
    />

    <!-- 底部工具栏：只显示图标 -->
    <div class="mt-3 flex items-center justify-between">
      <!-- 左侧功能图标组（当前仅样式，无具体交互） -->
      <div class="flex items-center gap-2">
        <!-- 新聊天 / 添加 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M10 4.16699V15.8337M4.16669 10.0003H15.8334"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>

        <!-- 聊天模式 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M5.00002 5.83366H15M5.00002 9.16699H11.6667M5.00002 15.0003L3.33335 15.8337V4.16699C3.33335 3.24652 4.07921 2.50066 4.99969 2.50066H15.0003C15.9208 2.50066 16.6667 3.24652 16.6667 4.16699V11.667C16.6667 12.5875 15.9208 13.3333 15.0003 13.3333H6.66669L5.00002 15.0003Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>

        <!-- 思考模式 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M10 3.33301C6.77834 3.33301 4.16669 5.94467 4.16669 9.16634C4.16669 11.3437 5.39419 13.2343 7.22252 14.212L7.50002 16.6663L9.16669 15.833L10.8334 16.6663L11.111 14.212C12.9394 13.2343 14.1667 11.3437 14.1667 9.16634C14.1667 5.94467 11.555 3.33301 8.33335 3.33301H10Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>

        <!-- 文件 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M6.66669 3.33301H11.25L14.1667 6.24967V15.833C14.1667 16.7535 13.4208 17.4993 12.5004 17.4993H6.66669C5.74621 17.4993 5.00035 16.7535 5.00035 15.833V4.99967C5.00035 4.0792 5.74621 3.33334 6.66669 3.33334Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M11.25 3.33301V6.24967H14.1667"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>

        <!-- 图片 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M4.99998 3.33301H15C15.9205 3.33301 16.6663 4.07887 16.6663 4.99935V15C16.6663 15.9205 15.9205 16.6663 15 16.6663H4.99998C4.0795 16.6663 3.33365 15.9205 3.33365 15V4.99935C3.33365 4.07887 4.0795 3.33301 4.99998 3.33301Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M7.08335 8.33301C7.7647 8.33301 8.33335 7.76437 8.33335 7.08301C8.33335 6.40166 7.7647 5.83301 7.08335 5.83301C6.402 5.83301 5.83335 6.40166 5.83335 7.08301C5.83335 7.76437 6.402 8.33301 7.08335 8.33301Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M16.6667 12.0837L13.3333 8.75033L6.66666 15.417"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>

        <!-- 知识库 / 文档 -->
        <button
          type="button"
          class="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
        >
          <svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-5">
            <path
              d="M4.16669 4.99967C4.16669 4.0792 4.91254 3.33334 5.83302 3.33334H14.1667V15.0003H5.83302C4.91254 15.0003 4.16669 15.7461 4.16669 16.6666V4.99967Z"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M7.5 6.66699H12.5M7.5 9.16699H10.8333"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>
      </div>

      <!-- 发送按钮：仅图标，保留原有交互逻辑 -->
      <button
        type="button"
        class="flex size-8 items-center justify-center rounded-full p-[5px] transition-colors"
        :class="
          canSend
            ? 'cursor-pointer bg-primary text-primary-foreground hover:bg-primary/90'
            : 'cursor-not-allowed bg-muted text-muted-foreground'
        "
        :disabled="!canSend"
        @click="handleSend"
      >
        <svg viewBox="0 0 22 22" fill="none" xmlns="http://www.w3.org/2000/svg" class="size-[22px]">
          <path
            d="M11 18V4M11 4L5 10M11 4L17 10"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
    </div>
  </div>
</template>
