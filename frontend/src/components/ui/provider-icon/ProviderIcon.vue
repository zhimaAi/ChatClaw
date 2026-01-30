<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { cn } from '@/lib/utils'

// 静态导入所有内置供应商图标（使用 ?url 后缀确保作为 URL 导入）
import openaiIcon from '@/assets/icons/providers/openai.svg?url'
import azureIcon from '@/assets/icons/providers/azure.svg?url'
import anthropicIcon from '@/assets/icons/providers/anthropic.svg?url'
import googleIcon from '@/assets/icons/providers/google.svg?url'
import deepseekIcon from '@/assets/icons/providers/deepseek.svg?url'
import zhipuIcon from '@/assets/icons/providers/zhipu.svg?url'
import qwenIcon from '@/assets/icons/providers/qwen.svg?url'
import doubaoIcon from '@/assets/icons/providers/doubao.svg?url'
import baiduIcon from '@/assets/icons/providers/baidu.svg?url'
import groqIcon from '@/assets/icons/providers/groq.svg?url'
import ollamaIcon from '@/assets/icons/providers/ollama.svg?url'
import defaultIcon from '@/assets/icons/providers/openai.svg?url'

interface Props {
  /**
   * 图标值，支持三种格式：
   * - 内置标识符：'openai', 'anthropic' 等
   * - Data URL：'data:image/svg+xml;base64,...' 或 'data:image/png;base64,...'
   * - 空值：显示默认图标
   */
  icon?: string
  /** 图标尺寸，默认 24 */
  size?: number
  /** 自定义 class */
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  icon: '',
  size: 24,
})

// 内置供应商图标映射
const builtinIcons: Record<string, string> = {
  openai: openaiIcon,
  azure: azureIcon,
  anthropic: anthropicIcon,
  google: googleIcon,
  deepseek: deepseekIcon,
  zhipu: zhipuIcon,
  qwen: qwenIcon,
  doubao: doubaoIcon,
  baidu: baiduIcon,
  groq: groqIcon,
  ollama: ollamaIcon,
}

/**
 * 判断是否为 Data URL（用户上传的自定义图标）
 */
function isDataUrl(str: string): boolean {
  return str.startsWith('data:')
}

/**
 * 判断是否为内置图标标识符
 */
function isBuiltinIcon(str: string): boolean {
  return str in builtinIcons
}

const iconSrc = computed(() => {
  const icon = props.icon?.trim()

  // 空值：使用默认图标
  if (!icon) {
    return defaultIcon
  }

  // Data URL（自定义上传的图标）
  if (isDataUrl(icon)) {
    return icon
  }

  // 内置图标标识符
  if (isBuiltinIcon(icon)) {
    return builtinIcons[icon]
  }

  // 未知值：使用默认图标
  return defaultIcon
})
</script>

<template>
  <img
    :src="iconSrc"
    :width="size"
    :height="size"
    alt="provider icon"
    :class="cn('inline-block shrink-0', props.class)"
  />
</template>
