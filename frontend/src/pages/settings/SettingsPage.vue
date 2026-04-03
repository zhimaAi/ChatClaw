<script setup lang="ts">
/**
 * 设置页面组件
 * 可在主窗口和独立设置窗口中复用
 */
import type { Component } from 'vue'
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 */
defineProps<{
  tabId?: string // 可选，因为设置页面也可能在独立窗口中使用
}>()
import SettingsSidebar from './components/SettingsSidebar.vue'
import GeneralSettings from './components/GeneralSettings.vue'
import ChatwikiSettings from './components/ChatwikiSettings.vue'
import ModelServiceSettings from './components/ModelServiceSettings.vue'
import SkillsSettings from './components/SkillsSettings.vue'
import MCPSettings from './components/MCPSettings.vue'
import SnapSettings from './components/SnapSettings.vue'
import ToolsSettings from './components/ToolsSettings.vue'
import OpenClawRuntimeSettings from './components/OpenClawRuntimeSettings.vue'
import AboutSettings from './components/AboutSettings.vue'
import { useAppStore, useSettingsStore, type SettingsMenuItem } from '@/stores'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const appStore = useAppStore()

watch(
  () => appStore.currentSystem,
  (system) => {
    if (
      system === 'openclaw' &&
      (settingsStore.activeMenu === 'chatwiki' ||
        settingsStore.activeMenu === 'mcp' ||
        settingsStore.activeMenu === 'openclawRuntime')
    ) {
      settingsStore.setActiveMenu('generalSettings')
    }
    if (system === 'chatclaw' && settingsStore.activeMenu === 'openclawRuntime') {
      settingsStore.setActiveMenu('generalSettings')
    }
  },
  { immediate: true }
)

// 菜单项对应的翻译 key
const menuLabelKeys: Record<SettingsMenuItem, string> = {
  modelService: 'settings.menu.modelService',
  generalSettings: 'settings.menu.generalSettings',
  openclawRuntime: 'settings.menu.openclawRuntime',
  skills: 'settings.menu.skills',
  mcp: 'settings.menu.mcp',
  snapSettings: 'settings.menu.snapSettings',
  tools: 'settings.menu.tools',
  chatwiki: 'settings.menu.chatwiki',
  about: 'settings.menu.about',
}

// 菜单项对应的内容组件（null 表示尚未实现）
const menuComponents: Record<SettingsMenuItem, Component | null> = {
  modelService: ModelServiceSettings,
  generalSettings: GeneralSettings,
  openclawRuntime: OpenClawRuntimeSettings,
  skills: SkillsSettings,
  mcp: MCPSettings,
  snapSettings: SnapSettings,
  tools: ToolsSettings,
  chatwiki: ChatwikiSettings,
  about: AboutSettings,
}

// 是否为全宽组件（不需要居中包装）
const isFullWidthComponent = computed(
  () => settingsStore.activeMenu === 'modelService' || settingsStore.activeMenu === 'mcp'
)

// 获取当前菜单的翻译文本
const activeMenuLabel = computed(() => t(menuLabelKeys[settingsStore.activeMenu]))

// 获取当前菜单对应的内容组件
const currentComponent = computed(() => menuComponents[settingsStore.activeMenu])
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <!-- 侧边栏导航 -->
    <SettingsSidebar />

    <!-- 内容区域 -->
    <main
      class="flex flex-1 flex-col overflow-auto"
      :class="!isFullWidthComponent && 'items-center py-8'"
    >
      <div
        v-if="currentComponent"
        class="w-full min-w-0"
        :class="
          settingsStore.activeMenu === 'openclawRuntime'
            ? 'mx-auto max-w-[88%] px-6 sm:px-8'
            : ''
        "
      >
        <component
          :is="currentComponent"
          v-bind="settingsStore.activeMenu === 'openclawRuntime' ? { figmaLayout: true } : {}"
        />
      </div>
      <!-- 占位内容：当其他菜单页面还没实现时显示 -->
      <div
        v-else
        class="mx-auto flex w-full max-w-[calc(100%-10rem)] lg:max-w-settings-card items-center justify-center rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
      >
        {{ activeMenuLabel }}
      </div>
    </main>
  </div>
</template>
