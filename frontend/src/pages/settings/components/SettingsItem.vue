<script setup lang="ts">
/**
 * 设置项组件
 * 用于设置卡片中的每一行设置项
 * 左侧显示标签（或自定义内容），右侧显示控件（通过 slot 传入）
 */
defineProps<{
  /** 设置项标签 */
  label?: string
  /** 是否显示底部边框，默认 true */
  bordered?: boolean
}>()
</script>

<template>
  <div
    class="flex flex-wrap items-center gap-x-4 gap-y-2 p-4 sm:flex-nowrap sm:justify-between"
    :class="bordered !== false && 'border-b border-border dark:border-white/10'"
  >
    <slot name="label">
      <span class="min-w-0 shrink-0 text-sm font-medium text-foreground">
        {{ label }}
      </span>
    </slot>
    <div class="min-w-0 shrink-0 sm:ml-auto">
      <slot />
    </div>
  </div>
</template>
