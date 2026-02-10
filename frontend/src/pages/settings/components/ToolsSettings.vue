<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Switch } from '@/components/ui/switch'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import { Window } from '@wailsio/runtime'

// 后端绑定
import { SettingsService, Category } from '@bindings/willclaw/internal/services/settings'
import { TrayService } from '@bindings/willclaw/internal/services/tray'
import { TextSelectionService } from '@bindings/willclaw/internal/services/textselection'
import { FloatingBallService } from '@bindings/willclaw/internal/services/floatingball'

const { t } = useI18n()

// 托盘设置状态
const showTrayIcon = ref(true)
const minimizeToTrayOnClose = ref(true)

// 悬浮窗设置状态
const showFloatingWindow = ref(false)

// 悬浮窗开关：快速连点时仅在“停手”后同步一次（避免频繁桥接/写库导致 UI 吞点击）
let floatingWindowDesired = showFloatingWindow.value
let floatingWindowApplied = showFloatingWindow.value
let suppressFloatingWindowSync = true
let floatingWindowSyncing = false
let floatingWindowPending = false
let floatingWindowDebounceTimer: ReturnType<typeof setTimeout> | null = null
const floatingClickCount = ref(0)

const debugFloatingToggle = () => {
  try {
    return localStorage.getItem('debugFloatingToggle') === '1'
  } catch {
    return false
  }
}

const logFloating = (...args: any[]) => {
  if (!debugFloatingToggle()) return

  console.log('[floating-toggle]', new Date().toISOString(), ...args)
}

const syncFloatingWindowDesired = async () => {
  if (floatingWindowSyncing) {
    floatingWindowPending = true
    logFloating('sync:already_in_flight -> pending', {
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
    return
  }
  floatingWindowSyncing = true
  floatingWindowPending = false
  const target = floatingWindowDesired
  logFloating('sync:start', {
    target,
    desired: floatingWindowDesired,
    applied: floatingWindowApplied,
    ui: showFloatingWindow.value,
  })
  try {
    await updateSetting('show_floating_window', String(target))
    try {
      await FloatingBallService.SetVisible(target)
    } catch (error) {
      console.error('Failed to update floating ball visibility:', error)
    }
    // macOS：悬浮球窗口 Show() 可能短暂抢占焦点，导致下一次点击仅用于“聚焦窗口”而不触发开关
    // 这里强制把焦点拉回当前窗口，避免出现“每隔一次点击无效”的现象
    try {
      await Window.Focus()
    } catch (e) {
      console.error('Failed to refocus window after floating toggle:', e)
    }
    floatingWindowApplied = target
    logFloating('sync:success', { applied: floatingWindowApplied })
  } catch (error) {
    console.error('Failed to update show_floating_window setting:', error)
    // 回滚到上一次成功状态
    floatingWindowDesired = floatingWindowApplied
    showFloatingWindow.value = floatingWindowApplied
    try {
      await FloatingBallService.SetVisible(floatingWindowApplied)
    } catch (e) {
      console.error('Failed to rollback floating ball visibility:', e)
    }
    try {
      await Window.Focus()
    } catch (e) {
      console.error('Failed to refocus window after rollback:', e)
    }
    logFloating('sync:failed -> rollback', {
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
  } finally {
    floatingWindowSyncing = false
    if (floatingWindowPending && floatingWindowDesired !== floatingWindowApplied) {
      logFloating('sync:pending_flush', {
        desired: floatingWindowDesired,
        applied: floatingWindowApplied,
        ui: showFloatingWindow.value,
      })
      void syncFloatingWindowDesired()
      return
    }
    logFloating('sync:end', {
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
  }
}

const scheduleFloatingWindowSync = (val: boolean) => {
  floatingWindowDesired = val
  if (floatingWindowDebounceTimer) {
    clearTimeout(floatingWindowDebounceTimer)
  }
  logFloating('schedule', {
    val,
    desired: floatingWindowDesired,
    applied: floatingWindowApplied,
    ui: showFloatingWindow.value,
  })
  floatingWindowDebounceTimer = setTimeout(() => {
    floatingWindowDebounceTimer = null
    logFloating('schedule:fire', {
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
    void syncFloatingWindowDesired()
  }, 160)
}

watch(
  showFloatingWindow,
  (val) => {
    if (suppressFloatingWindowSync) return
    logFloating('watch:model', {
      val,
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
    scheduleFloatingWindowSync(val)
  },
  { flush: 'sync' }
)

// 划词搜索设置状态
const enableSelectionSearch = ref(false)

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

    // 同步悬浮球显示状态（仅在不一致时才调用，避免进入页面就触发“重新定位”）
    try {
      const currentVisible = await FloatingBallService.IsVisible()
      if (currentVisible !== showFloatingWindow.value) {
        await FloatingBallService.SetVisible(showFloatingWindow.value)
      }
      floatingWindowDesired = showFloatingWindow.value
      floatingWindowApplied = showFloatingWindow.value
      suppressFloatingWindowSync = false
    } catch (error) {
      console.error('Failed to sync floating ball visibility on load:', error)
      suppressFloatingWindowSync = false
    }

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
const handleFloatingWindowChange = async (_val: boolean) => {
  // 兼容旧模板写法：实际切换由 v-model 触发，后端同步由 watch 处理
}

const handleFloatingWindowClickCapture = () => {
  floatingClickCount.value += 1
  logFloating('click:capture', {
    n: floatingClickCount.value,
    ui_before: showFloatingWindow.value,
    desired: floatingWindowDesired,
    applied: floatingWindowApplied,
    syncing: floatingWindowSyncing,
  })
}

// 处理划词搜索开关变化
const handleSelectionSearchChange = async (val: boolean) => {
  const prev = enableSelectionSearch.value
  enableSelectionSearch.value = val
  try {
    await updateSetting('enable_selection_search', String(val))
    // Sync with backend text selection service
    await TextSelectionService.SyncFromSettings()
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
        <div @click.capture="handleFloatingWindowClickCapture">
          <Switch v-model="showFloatingWindow" />
        </div>
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
