<script setup lang="ts">
/**
 * 主布局组件
 * 整合标题栏、侧边导航和主内容区
 */
import TitleBar from './TitleBar.vue'
import SideNav from './SideNav.vue'
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden bg-background">
    <!-- 标题栏/标签栏 -->
    <TitleBar />

    <!-- 主体区域 -->
    <div class="flex flex-1 overflow-hidden">
      <!-- 左侧导航 -->
      <SideNav />

      <!-- 主内容区 -->
      <main class="relative flex-1 overflow-hidden">
        <slot />
      </main>
    </div>
  </div>
</template>
