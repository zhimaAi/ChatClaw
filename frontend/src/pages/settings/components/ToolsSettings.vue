<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Switch } from '@/components/ui/switch'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

// 后端绑定
import { SettingsService, Category } from '../../../../bindings/willchat/internal/services/settings'

const { t } = useI18n()

// 托盘设置状态
const showTrayIcon = ref(true)
const minimizeToTrayOnClose = ref(true)

// 悬浮窗设置状态
const showFloatingWindow = ref(true)

// 划词搜索设置状态
const enableSelectionSearch = ref(true)

// 加载设置
const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategoryTools)
    settings.forEach((setting) => {
      switch (setting.key) {
        case 'show_tray_icon':
          showTrayIcon.value = setting.value === 'true'
          break
        case 'minimize_to_tray_on_close':
          minimizeToTrayOnClose.value = setting.value === 'true'
          break
        case 'show_floating_window':
          showFloatingWindow.value = setting.value === 'true'
          break
        case 'enable_selection_search':
          enableSelectionSearch.value = setting.value === 'true'
          break
      }
    })
  } catch (error) {
    console.error('Failed to load tools settings:', error)
  }
}

// 更新设置
const updateSetting = async (key: string, value: string) => {
  try {
    await SettingsService.SetValue(key, value)
  } catch (error) {
    console.error(`Failed to update setting ${key}:`, error)
  }
}

// 处理托盘图标开关变化
const handleTrayIconChange = (val: boolean) => {
  showTrayIcon.value = val
  void updateSetting('show_tray_icon', String(val))
}

// 处理最小化到托盘开关变化
const handleMinimizeToTrayChange = (val: boolean) => {
  minimizeToTrayOnClose.value = val
  void updateSetting('minimize_to_tray_on_close', String(val))
}

// 处理悬浮窗开关变化
const handleFloatingWindowChange = (val: boolean) => {
  showFloatingWindow.value = val
  void updateSetting('show_floating_window', String(val))
}

// 处理划词搜索开关变化
const handleSelectionSearchChange = (val: boolean) => {
  enableSelectionSearch.value = val
  void updateSetting('enable_selection_search', String(val))
}

// 页面加载时获取设置
onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- 托盘设置卡片 -->
    <SettingsCard :title="t('settings.tools.tray.title')">
      <!-- 显示托盘图标 -->
      <SettingsItem :label="t('settings.tools.tray.showIcon')">
        <Switch :model-value="showTrayIcon" @update:model-value="handleTrayIconChange" />
      </SettingsItem>

      <!-- 关闭时最小化到托盘 -->
      <SettingsItem :label="t('settings.tools.tray.minimizeOnClose')" :bordered="false">
        <Switch :model-value="minimizeToTrayOnClose" @update:model-value="handleMinimizeToTrayChange" />
      </SettingsItem>
    </SettingsCard>

    <!-- 悬浮窗设置卡片 -->
    <SettingsCard :title="t('settings.tools.floatingWindow.title')">
      <!-- 显示悬浮窗 -->
      <SettingsItem :label="t('settings.tools.floatingWindow.show')" :bordered="false">
        <Switch :model-value="showFloatingWindow" @update:model-value="handleFloatingWindowChange" />
      </SettingsItem>
    </SettingsCard>

    <!-- 划词搜索设置卡片 -->
    <SettingsCard :title="t('settings.tools.selectionSearch.title')">
      <!-- 划词搜索 -->
      <SettingsItem :label="t('settings.tools.selectionSearch.enable')" :bordered="false">
        <Switch :model-value="enableSelectionSearch" @update:model-value="handleSelectionSearchChange" />
      </SettingsItem>
    </SettingsCard>
  </div>
</template>
