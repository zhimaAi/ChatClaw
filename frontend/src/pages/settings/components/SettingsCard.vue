<script setup lang="ts">
/**
 * 设置卡片组件
 * 用于包裹设置页面的各个设置区块
 * 宽度固定 530px，高度根据内容自动延伸
 */
defineProps<{
  /** 卡片标题 */
  title: string
}>()
</script>

<template>
  <div
    class="w-settings-card flex-none rounded-2xl border border-border bg-card shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
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
