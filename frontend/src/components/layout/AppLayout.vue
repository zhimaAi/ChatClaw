<script setup lang="ts">
/**
 * 应用主布局组件
 * 提供基础的页面结构框架
 */
defineProps<{
  /** 是否显示侧边栏 */
  showSidebar?: boolean
}>()
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-background text-foreground">
    <!-- 侧边栏插槽 -->
    <aside v-if="showSidebar" class="flex-shrink-0">
      <slot name="sidebar" />
    </aside>

    <!-- 主内容区 -->
    <main class="flex flex-1 flex-col overflow-hidden">
      <!-- 头部插槽 -->
      <header v-if="$slots.header" class="flex-shrink-0">
        <slot name="header" />
      </header>

      <!-- 内容区 -->
      <div class="flex-1 overflow-auto">
        <slot />
      </div>

      <!-- 底部插槽 -->
      <footer v-if="$slots.footer" class="flex-shrink-0">
        <slot name="footer" />
      </footer>
    </main>
  </div>
</template>
