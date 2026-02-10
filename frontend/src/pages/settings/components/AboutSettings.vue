<script setup lang="ts">
/**
 * About settings component.
 * Update dialog is rendered in App.vue for global access.
 */
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight } from 'lucide-vue-next'
import { BrowserService } from '@bindings/willclaw/internal/services/browser'
import { AppService } from '@bindings/willclaw/internal/services/app'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'
import { UpdaterService } from '@bindings/willclaw/internal/services/updater'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { Events } from '@wailsio/runtime'
import { useAppStore } from '@/stores'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import LogoIcon from '@/assets/images/logo.svg'

const { t } = useI18n()
const appStore = useAppStore()

// Official website
const OFFICIAL_WEBSITE = 'https://github.com/zhimaAi/WillClaw'

// Application version
const appVersion = ref('...')

// Check update loading state
const isCheckingUpdate = ref(false)

// Auto update toggle
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

// Open website
async function handleOpenWebsite() {
  try {
    await BrowserService.OpenURL(OFFICIAL_WEBSITE)
  } catch (error) {
    console.error('Failed to open website:', error)
  }
}

// Check for update — delegates to App.vue via frontend event
async function handleCheckUpdate() {
  if (isCheckingUpdate.value) return
  isCheckingUpdate.value = true
  try {
    const result = await UpdaterService.CheckForUpdate()
    if (result && result.has_update) {
      // Mark update available in global store (for badge display)
      appStore.hasAvailableUpdate = true
      // Notify App.vue to open the update dialog
      Events.Emit('update:show-dialog', {
        mode: 'new-version',
        version: result.latest_version,
        release_notes: result.release_notes || '',
      })
    } else {
      appStore.hasAvailableUpdate = false
      toast.success(t('settings.about.alreadyLatest'))
    }
  } catch (error) {
    console.error('Failed to check for update:', error)
    toast.error(t('settings.about.checkFailed'))
  } finally {
    isCheckingUpdate.value = false
  }
}

// Toggle auto update
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
    <!-- Application info area -->
    <div class="flex items-center gap-5 border-b border-border p-4 dark:border-white/10">
      <!-- Logo -->
      <div
        class="flex size-icon-box shrink-0 items-center justify-center rounded-icon-box border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
      >
        <LogoIcon class="size-icon-lg" />
      </div>

      <!-- App name and copyright -->
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

      <!-- Check update button -->
      <div class="relative inline-flex">
        <Button
          variant="outline"
          size="sm"
          :disabled="isCheckingUpdate"
          @click="handleCheckUpdate"
        >
          {{ isCheckingUpdate ? t('settings.about.checkingUpdate') : t('settings.about.checkUpdate') }}
        </Button>
        <!-- Badge indicating a new version is available -->
        <span
          v-if="appStore.hasAvailableUpdate"
          class="absolute -right-1 -top-1 size-2.5 rounded-full bg-foreground/70"
        />
      </div>
    </div>

    <!-- Official website -->
    <SettingsItem :label="t('settings.about.officialWebsite')">
      <button
        class="inline-flex cursor-pointer items-center gap-1 text-sm text-primary hover:opacity-80"
        @click="handleOpenWebsite"
      >
        {{ t('settings.about.view') }}
        <ChevronRight class="size-4" />
      </button>
    </SettingsItem>

    <!-- Auto update toggle -->
    <SettingsItem :label="t('settings.about.autoUpdate')" :bordered="false">
      <Switch :model-value="autoUpdate" @update:model-value="handleAutoUpdateChange" />
    </SettingsItem>
  </SettingsCard>
</template>
