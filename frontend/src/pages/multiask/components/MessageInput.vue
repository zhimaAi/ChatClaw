<script setup lang="ts">
/**
 * 消息输入框组件
 * 仅支持文本输入和发送
 */
import { computed } from 'vue'
import { cn } from '@/lib/utils'

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
</script>

<template>
  <div class="flex w-full flex-col gap-4 rounded-2xl border-2 border-border bg-background px-3 py-2.5 shadow-sm">
    <!-- 输入区域 -->
    <textarea
      v-model="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      rows="2"
      class="min-h-[48px] w-full resize-none bg-transparent text-base text-foreground outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
      @keydown="handleKeydown"
    />

    <!-- 工具栏 -->
    <div class="flex items-center justify-end">
      <!-- 发送按钮 -->
      <button
        type="button"
        :class="cn(
          'flex size-8 items-center justify-center rounded-full p-[5px] transition-colors',
          canSend
            ? 'cursor-pointer bg-primary text-primary-foreground hover:bg-primary/90'
            : 'cursor-not-allowed bg-[#d8dde5] text-white'
        )"
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
