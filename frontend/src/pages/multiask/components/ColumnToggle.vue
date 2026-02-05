<script setup lang="ts">
/**
 * 分栏切换组件
 * 支持 1/2/3 列布局切换
 */
import { cn } from '@/lib/utils'
import MainPanelIcon from '@/assets/icons/main-panel.svg'
import TwoPanelIcon from '@/assets/icons/two-panel.svg'
import ThreePanelIcon from '@/assets/icons/three-panel.svg'

const modelValue = defineModel<number>({ default: 3 })

const columnOptions = [
  { value: 1, icon: MainPanelIcon, title: '1 列布局' },
  { value: 2, icon: TwoPanelIcon, title: '2 列布局' },
  { value: 3, icon: ThreePanelIcon, title: '3 列布局' },
] as const
</script>

<template>
  <div class="flex shrink-0 items-center gap-0.5 rounded-full bg-muted p-0.5">
    <button
      v-for="option in columnOptions"
      :key="option.value"
      type="button"
      :class="cn(
        'flex size-7 cursor-pointer items-center justify-center rounded-full transition-colors',
        modelValue === option.value
          ? 'bg-background shadow-sm'
          : 'hover:bg-accent/50'
      )"
      :title="option.title"
      @click="modelValue = option.value"
    >
      <component
        :is="option.icon"
        :class="cn(
          'size-[18px] transition-colors',
          modelValue === option.value ? 'text-foreground' : 'text-muted-foreground'
        )"
      />
    </button>
  </div>
</template>

<style scoped>
/* SVG 图标颜色跟随 currentColor */
button :deep(svg) {
  stroke: currentColor;
}
button :deep(svg rect),
button :deep(svg path) {
  stroke: currentColor;
}
</style>
