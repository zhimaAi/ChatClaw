<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MainLayout } from '@/components/layout'
import { useNavigationStore } from '@/stores'
import SettingsPage from '@/pages/settings/SettingsPage.vue'

const { t } = useI18n()
const navigationStore = useNavigationStore()

/**
 * 当前激活的标签页
 */
const activeTab = computed(() => navigationStore.activeTab)

/**
 * 是否显示设置页面
 */
const showSettings = computed(() => activeTab.value?.module === 'settings')

/**
 * 默认至少保持 1 个标签页：
 * - 启动时若没有标签页，自动打开一个 AI助手
 * - 当用户关闭到 0 个标签页时，自动再打开一个 AI助手
 */
watch(
  () => navigationStore.tabs.length,
  (len) => {
    if (len === 0) {
      navigationStore.navigateToModule('assistant')
    }
  },
  { immediate: true }
)
</script>

<template>
  <MainLayout>
    <!-- 设置页面 -->
    <SettingsPage v-if="showSettings" />

    <!-- 主内容区域 - 显示当前模块的占位内容 -->
    <div v-else class="flex h-full w-full items-center justify-center bg-background">
      <div class="flex flex-col items-center gap-4">
        <!-- 显示当前标签页标题，如果没有标签页则显示提示 -->
        <template v-if="activeTab">
          <h1 class="text-2xl font-semibold text-foreground">
            {{ activeTab.titleKey ? t(activeTab.titleKey) : activeTab.title }}
          </h1>
          <p class="text-muted-foreground">
            {{ t('app.title') }}
          </p>
        </template>
        <template v-else>
          <h1 class="text-2xl font-semibold text-foreground">
            {{ t('app.title') }}
          </h1>
          <p class="text-muted-foreground">
            {{ t('nav.assistant') }}
          </p>
        </template>
      </div>
    </div>
  </MainLayout>
</template>
