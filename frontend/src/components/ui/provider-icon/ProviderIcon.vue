<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

type SvgComponent = any
import { computed } from 'vue'
import { cn } from '@/lib/utils'

// 静态导入所有内置供应商图标（作为 Vue 组件，支持 currentColor）
import OpenaiIcon from '@/assets/icons/providers/openai.svg'
import AzureIcon from '@/assets/icons/providers/azure.svg'
import AnthropicIcon from '@/assets/icons/providers/anthropic.svg'
import GoogleIcon from '@/assets/icons/providers/google.svg'
import DeepseekIcon from '@/assets/icons/providers/deepseek.svg'
import ZhipuIcon from '@/assets/icons/providers/zhipu.svg'
import QwenIcon from '@/assets/icons/providers/qwen.svg'
import DoubaoIcon from '@/assets/icons/providers/doubao.svg'
import BaiduIcon from '@/assets/icons/providers/baidu.svg'
import GrokIcon from '@/assets/icons/providers/grok.svg'
import OllamaIcon from '@/assets/icons/providers/ollama.svg'
import ChatwikiIcon from '@/assets/icons/providers/chatwiki.svg'

// AI 模型图标（用于多问页面）
import ChatgptModelIcon from '@/assets/icons/models/chatgpt-icon.svg'
import ClaudeModelIcon from '@/assets/icons/models/claude-icon.svg'
import DeepseekModelIcon from '@/assets/icons/models/deepseek-icon.svg'
import DoubaoModelIcon from '@/assets/icons/models/doubao-icon.svg'
import GeminiModelIcon from '@/assets/icons/models/gemini-icon.svg'
import QwenModelIcon from '@/assets/icons/models/qwen-icon.svg'

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

// 内置供应商图标映射（Vue 组件）
const builtinIcons: Record<string, SvgComponent> = {
  openai: OpenaiIcon,
  azure: AzureIcon,
  anthropic: AnthropicIcon,
  google: GoogleIcon,
  deepseek: DeepseekIcon,
  zhipu: ZhipuIcon,
  qwen: QwenIcon,
  doubao: DoubaoIcon,
  baidu: BaiduIcon,
  grok: GrokIcon,
  ollama: OllamaIcon,
  chatwiki: ChatwikiIcon,
  // AI 模型图标（用于多问页面，使用 model- 前缀区分）
  'model-chatgpt': ChatgptModelIcon,
  'model-claude': ClaudeModelIcon,
  'model-deepseek': DeepseekModelIcon,
  'model-doubao': DoubaoModelIcon,
  'model-gemini': GeminiModelIcon,
  'model-qwen': QwenModelIcon,
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

// 获取内置图标组件
const iconComponent = computed(() => {
  const icon = props.icon?.trim()
  if (icon && isBuiltinIcon(icon)) {
    return builtinIcons[icon]
  }
  return builtinIcons.openai // 默认使用 OpenAI 图标
})

// 获取 Data URL 图标
const iconUrl = computed(() => {
  const icon = props.icon?.trim()
  if (icon && isDataUrl(icon)) {
    return icon
  }
  return null
})

// 是否使用组件渲染（内置图标）
const useComponent = computed(() => {
  const icon = props.icon?.trim()
  return !icon || isBuiltinIcon(icon)
})
</script>

<template>
  <!-- 内置图标：作为 Vue 组件渲染，支持 currentColor；外层固定尺寸避免 SVG 自身 1em 等导致放大 -->
  <span
    v-if="useComponent"
    class="inline-block shrink-0"
    :style="{ width: size + 'px', height: size + 'px' }"
  >
    <component
      :is="iconComponent"
      :width="size"
      :height="size"
      :class="cn('block h-full w-full text-foreground', props.class)"
    />
  </span>
  <!-- Data URL 图标：作为 img 渲染 -->
  <img
    v-else
    :src="iconUrl || ''"
    :width="size"
    :height="size"
    alt="provider icon"
    :class="cn('inline-block shrink-0', props.class)"
  />
</template>
