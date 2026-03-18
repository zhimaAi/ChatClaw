<script setup lang="ts">
/**
 * AI 模型选择组件
 * 水平展示可选的 AI 模型卡片，支持多选
 */
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
}>()

/**
 * 检查模型是否被选中
 */
const isSelected = (modelId: string) => {
  return props.selectedIds.includes(modelId)
}

const handleClick = (modelId: string) => {
  emit('toggle', modelId)
}
</script>

<template>
  <div class="relative -mx-2 flex items-center gap-2 overflow-x-auto px-2 py-2 select-none">
    <button
      v-for="model in models"
      :key="model.id"
      type="button"
      :class="
        cn(
          'relative flex h-[62px] w-[54px] shrink-0 cursor-pointer flex-col items-center gap-1 rounded-md p-1 transition-all',
          isSelected(model.id) ? 'bg-[#f5f5f5] dark:bg-muted' : 'bg-background hover:bg-muted/50'
        )
      "
      @click="handleClick(model.id)"
    >
      <!-- 选中标记（左上角勾选图标） -->
      <SelectedIcon
        v-if="isSelected(model.id)"
        class="absolute left-0 top-0 size-6 rounded-tl-md"
      />

      <!-- 模型图标 -->
      <div
        class="flex size-8 items-center justify-center rounded-md border border-border bg-background"
      >
        <ProviderIcon :icon="model.icon" :size="24" />
      </div>

      <!-- 模型名称 -->
      <span class="w-full truncate text-center text-xs text-muted-foreground">
        {{ model.displayName || model.name }}
      </span>
    </button>
  </div>
</template>
