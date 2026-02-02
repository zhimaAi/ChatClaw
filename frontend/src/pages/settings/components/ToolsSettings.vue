<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Switch } from '@/components/ui/switch'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

// 后端绑定
import { SettingsService, Category } from '@bindings/willchat/internal/services/settings'
import { TrayService } from '@bindings/willchat/internal/services/tray'

const { t } = useI18n()

// 托盘设置状态
const showTrayIcon = ref(true)
const minimizeToTrayOnClose = ref(true)

// 悬浮窗设置状态
const showFloatingWindow = ref(true)

// 划词搜索设置状态
const enableSelectionSearch = ref(true)

// 布尔设置映射表
const boolSettingsMap: Record<string, { value: boolean }> = {
  show_tray_icon: showTrayIcon,
  minimize_to_tray_on_close: minimizeToTrayOnClose,
  show_floating_window: showFloatingWindow,
  enable_selection_search: enableSelectionSearch,
}

// 加载设置
const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategoryTools)
    settings.forEach((setting) => {
      const boolRef = boolSettingsMap[setting.key]
      if (boolRef) {
        boolRef.value = setting.value === 'true'
      }
    })

    // 安全兜底：没有托盘图标时，不允许启用“关闭时最小化到托盘”（否则可能无法找回窗口）
    if (!showTrayIcon.value && minimizeToTrayOnClose.value) {
      minimizeToTrayOnClose.value = false
      await updateSetting('minimize_to_tray_on_close', 'false')
      try {
        await TrayService.SetMinimizeToTrayEnabled(false)
      } catch (error) {
        console.error('Failed to sync minimize-to-tray cache on load:', error)
      }
    }
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
    throw error
  }
}

// 处理托盘图标开关变化
const handleTrayIconChange = async (val: boolean) => {
  const prevShowTrayIcon = showTrayIcon.value
  const prevMinimizeToTray = minimizeToTrayOnClose.value

  // 安全联动：关闭托盘图标时，强制关闭“关闭时最小化到托盘”
  showTrayIcon.value = val
  if (!val && minimizeToTrayOnClose.value) {
    minimizeToTrayOnClose.value = false
  }

  try {
    await updateSetting('show_tray_icon', String(val))

    if (!val && prevMinimizeToTray) {
      await updateSetting('minimize_to_tray_on_close', 'false')
      try {
        await TrayService.SetMinimizeToTrayEnabled(false)
      } catch (error) {
        console.error('Failed to sync minimize-to-tray cache:', error)
      }
    }

    // 立即更新托盘图标可见性（后端也会做兜底，避免不可恢复状态）
    try {
      await TrayService.SetVisible(val)
    } catch (error) {
      console.error('Failed to update tray visibility:', error)
    }
  } catch {
    // 回滚 UI
    showTrayIcon.value = prevShowTrayIcon
    minimizeToTrayOnClose.value = prevMinimizeToTray
  }
}

// 处理最小化到托盘开关变化
const handleMinimizeToTrayChange = async (val: boolean) => {
  const prev = minimizeToTrayOnClose.value

  // 没有托盘图标时不允许启用（避免窗口隐藏后无法恢复）
  if (!showTrayIcon.value && val) {
    minimizeToTrayOnClose.value = false
    return
  }

  minimizeToTrayOnClose.value = val
  try {
    await updateSetting('minimize_to_tray_on_close', String(val))

    // 同步更新后端内存缓存，避免窗口关闭时查库
    try {
      await TrayService.SetMinimizeToTrayEnabled(val)
    } catch (error) {
      console.error('Failed to update minimize-to-tray setting:', error)
    }
  } catch {
    minimizeToTrayOnClose.value = prev
  }
}

// 处理悬浮窗开关变化
const handleFloatingWindowChange = async (val: boolean) => {
  const prev = showFloatingWindow.value
  showFloatingWindow.value = val
  try {
    await updateSetting('show_floating_window', String(val))
  } catch {
    showFloatingWindow.value = prev
  }
}

// 处理划词搜索开关变化
const handleSelectionSearchChange = async (val: boolean) => {
  const prev = enableSelectionSearch.value
  enableSelectionSearch.value = val
  try {
    await updateSetting('enable_selection_search', String(val))
  } catch {
    enableSelectionSearch.value = prev
  }
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
        <Switch
          :model-value="minimizeToTrayOnClose"
          @update:model-value="handleMinimizeToTrayChange"
        />
      </SettingsItem>
    </SettingsCard>

    <!-- 悬浮窗设置卡片 -->
    <SettingsCard :title="t('settings.tools.floatingWindow.title')">
      <!-- 显示悬浮窗 -->
      <SettingsItem :label="t('settings.tools.floatingWindow.show')" :bordered="false">
        <Switch
          :model-value="showFloatingWindow"
          @update:model-value="handleFloatingWindowChange"
        />
      </SettingsItem>
    </SettingsCard>

    <!-- 划词搜索设置卡片 -->
    <SettingsCard :title="t('settings.tools.selectionSearch.title')">
      <!-- 划词搜索 -->
      <SettingsItem :label="t('settings.tools.selectionSearch.enable')" :bordered="false">
        <Switch
          :model-value="enableSelectionSearch"
          @update:model-value="handleSelectionSearchChange"
        />
      </SettingsItem>
    </SettingsCard>
  </div>
</template>
