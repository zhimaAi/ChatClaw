<script setup lang="ts">
/**
 * 关于我们设置组件
 */
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
import { BrowserService } from '@bindings/willchat/internal/services/browser'
import { AppService } from '@bindings/willchat/internal/services/app'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import LogoIcon from '@/assets/images/logo.svg'

const { t } = useI18n()

// 官网地址
const OFFICIAL_WEBSITE = 'https://willchat.chatwiki.com'

// 应用版本
const appVersion = ref('...')

onMounted(async () => {
  try {
    const version = await AppService.GetVersion()
    appVersion.value = version.startsWith('v') ? version : `v${version}`
  } catch (error) {
    console.error('Failed to get version:', error)
    appVersion.value = 'unknown'
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
    </div>

    <!-- 官方网站链接 -->
    <SettingsItem :label="t('settings.about.officialWebsite')" :bordered="false">
      <button
        class="inline-flex cursor-pointer items-center gap-1 text-sm text-primary hover:opacity-80"
        @click="handleOpenWebsite"
      >
        {{ t('settings.about.view') }}
        <ChevronRight class="size-4" />
      </button>
    </SettingsItem>
  </SettingsCard>
</template>
