<script setup lang="ts">
/**
 * 设置页面组件
 * 可在主窗口和独立设置窗口中复用
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SettingsSidebar from './components/SettingsSidebar.vue'
import GeneralSettings from './components/GeneralSettings.vue'
import { useSettingsStore, type SettingsMenuItem } from './stores/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()

// 菜单项对应的翻译 key
const menuLabelKeys: Record<SettingsMenuItem, string> = {
  modelService: 'settings.menu.modelService',
  generalSettings: 'settings.menu.generalSettings',
  snapSettings: 'settings.menu.snapSettings',
  tools: 'settings.menu.tools',
  about: 'settings.menu.about',
}

// 获取当前菜单的翻译文本
const activeMenuLabel = computed(() => t(menuLabelKeys[settingsStore.activeMenu]))

// 根据当前菜单返回对应的内容组件
const currentComponent = computed(() => {
  switch (settingsStore.activeMenu) {
    case 'generalSettings':
      return GeneralSettings
    // 其他菜单页面后续实现
    default:
      return null
  }
})
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <!-- 侧边栏导航 -->
    <SettingsSidebar />

    <!-- 内容区域 -->
    <main class="flex flex-1 flex-col items-center overflow-auto py-8">
      <component :is="currentComponent" v-if="currentComponent" />
      <!-- 占位内容：当其他菜单页面还没实现时显示 -->
      <div
        v-else
        class="flex w-[530px] items-center justify-center rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm"
      >
        {{ activeMenuLabel }}
      </div>
    </main>
  </div>
</template>
