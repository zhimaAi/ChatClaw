<script setup lang="ts">
/**
 * 设置卡片组件
 * 用于包裹设置页面的各个设置区块
 * 默认自适应父容器宽度；fullWidth 时强制占满父容器（如 OpenClaw 管家页全宽场景）
 */
defineProps<{
  /** 卡片标题 */
  title: string
  /** When true, card stretches to parent width instead of auto width */
  fullWidth?: boolean
}>()
</script>

<template>
  <div
    :class="[
      fullWidth
        ? 'w-full min-w-0'
        : 'mx-auto w-full max-w-[calc(100%-10rem)] lg:max-w-settings-card',
      'rounded-2xl border border-border bg-card shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5',
    ]"
  >
    <!-- 标题区域 -->
    <div
      class="flex items-center justify-between rounded-t-2xl border-b border-border bg-muted/30 p-4 dark:border-white/10 dark:bg-white/5"
    >
      <h2 class="text-base font-semibold text-foreground">
        {{ title }}
      </h2>
      <slot name="header-right" />
    </div>

    <!-- 内容区域 -->
    <slot />
  </div>
</template>
