<script setup lang="ts">
/**
 * 关于我们设置组件
 */
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
import { BrowserService } from '@bindings/willchat/internal/services/browser'
import { AppService } from '@bindings/willchat/internal/services/app'
import { SettingsService } from '@bindings/willchat/internal/services/settings'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import LogoIcon from '@/assets/images/logo.svg'

const { t } = useI18n()

// 官网地址
const OFFICIAL_WEBSITE = 'https://github.com/zhimaAi/WillChat'

// 应用版本
const appVersion = ref('...')

// 检查更新状态
const isCheckingUpdate = ref(false)

// 自动更新开关状态
const autoUpdate = ref(true)

onMounted(async () => {
  try {
    const version = await AppService.GetVersion()
    appVersion.value = version.startsWith('v') ? version : `v${version}`
  } catch (error) {
    console.error('Failed to get version:', error)
    appVersion.value = 'unknown'
  }

  // Load auto update setting
  try {
    const setting = await SettingsService.Get('auto_update')
    if (setting) {
      autoUpdate.value = setting.value === 'true'
    }
  } catch {
    // Use default value if setting not found
  }
})

// 打开官网
async function handleOpenWebsite() {
  try {
    await BrowserService.OpenURL(OFFICIAL_WEBSITE)
  } catch (error) {
    console.error('Failed to open website:', error)
  }
}

// 检查更新
async function handleCheckUpdate() {
  if (isCheckingUpdate.value) return
  isCheckingUpdate.value = true
  try {
    await AppService.CheckForUpdate()
    toast.success(t('settings.about.alreadyLatest'))
  } catch (error) {
    console.error('Failed to check for update:', error)
  } finally {
    isCheckingUpdate.value = false
  }
}

// 切换自动更新
async function handleAutoUpdateChange(value: boolean) {
  autoUpdate.value = value
  try {
    await SettingsService.SetValue('auto_update', String(value))
  } catch (error) {
    console.error('Failed to save auto update setting:', error)
    // Revert on failure
    autoUpdate.value = !value
  }
}
</script>

<template>
  <SettingsCard :title="t('settings.about.title')">
    <!-- 应用信息区域 -->
    <div class="flex items-center gap-5 border-b border-border p-4 dark:border-white/10">
      <!-- Logo -->
      <div
        class="flex size-icon-box shrink-0 items-center justify-center rounded-icon-box border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
      >
        <LogoIcon class="size-icon-lg" />
      </div>

      <!-- 应用名称和版权信息 -->
      <div class="flex flex-1 flex-col items-start gap-1">
        <span class="text-sm font-medium text-foreground">
          {{ t('settings.about.appName') }}
        </span>
        <span class="text-xs text-muted-foreground">
          {{ t('settings.about.copyright') }}
        </span>
        <div
          class="inline-flex items-center rounded-xl bg-muted px-2 py-0.5 text-xs text-muted-foreground"
        >
          {{ appVersion }}
        </div>
      </div>

      <!-- 检查更新按钮 -->
      <Button
        variant="outline"
        size="sm"
        :disabled="isCheckingUpdate"
        @click="handleCheckUpdate"
      >
        {{ isCheckingUpdate ? t('settings.about.checkingUpdate') : t('settings.about.checkUpdate') }}
      </Button>
    </div>

    <!-- 官方网站链接 -->
    <SettingsItem :label="t('settings.about.officialWebsite')">
      <button
        class="inline-flex cursor-pointer items-center gap-1 text-sm text-primary hover:opacity-80"
        @click="handleOpenWebsite"
      >
        {{ t('settings.about.view') }}
        <ChevronRight class="size-4" />
      </button>
    </SettingsItem>

    <!-- 自动更新开关 -->
    <SettingsItem :label="t('settings.about.autoUpdate')" :bordered="false">
      <Switch :model-value="autoUpdate" @update:model-value="handleAutoUpdateChange" />
    </SettingsItem>
  </SettingsCard>
</template>
