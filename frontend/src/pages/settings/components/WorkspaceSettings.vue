<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle, FolderOpen, ShieldCheck, Monitor, Globe } from 'lucide-vue-next'
import { Dialogs } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useAppStore } from '@/stores'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)

const sandboxMode = ref('codex')
const sandboxNetwork = ref(false)
const workDir = ref('')
const defaultWorkDir = ref('')

const displayWorkDir = computed(() => {
  return workDir.value || defaultWorkDir.value || '~/.chatclaw'
})

const modeDescription = computed(() => {
  return sandboxMode.value === 'codex'
    ? t('settings.workspace.codexDesc')
    : t('settings.workspace.nativeDesc')
})

const loadData = async () => {
  loading.value = true
  try {
    const [modeSetting, dirSetting, networkSetting] = await Promise.all([
      SettingsService.Get('workspace_sandbox_mode'),
      SettingsService.Get('workspace_work_dir'),
      SettingsService.Get('workspace_sandbox_network'),
    ])

    sandboxMode.value = modeSetting?.value || 'codex'
    workDir.value = dirSetting?.value || ''
    sandboxNetwork.value = networkSetting?.value === 'true'
  } catch (error) {
    console.error('Failed to load workspace settings:', error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadData()
})

const handleSelectDir = async () => {
  if (!appStore.isGUIMode) return
  try {
    const result = await Dialogs.OpenFile({
      Title: t('settings.workspace.selectDir'),
      CanChooseFiles: false,
      CanChooseDirectories: true,
      AllowsMultipleSelection: false,
    })
    if (result && typeof result === 'string') {
      workDir.value = result
    } else if (Array.isArray(result) && result.length > 0) {
      workDir.value = result[0]
    }
  } catch (error) {
    console.error('Failed to select directory:', error)
  }
}

const handleSave = async () => {
  if (saving.value) return
  saving.value = true

  try {
    await Promise.all([
      SettingsService.SetValue('workspace_sandbox_mode', sandboxMode.value),
      SettingsService.SetValue('workspace_work_dir', workDir.value),
      SettingsService.SetValue('workspace_sandbox_network', sandboxNetwork.value ? 'true' : 'false'),
    ])
    toast.success(t('settings.workspace.saved'))
  } catch (error) {
    console.error('Failed to save workspace settings:', error)
    toast.error(getErrorMessage(error) || t('settings.workspace.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <SettingsCard :title="t('settings.workspace.title')">
    <div v-if="loading" class="flex items-center justify-center py-8">
      <LoaderCircle class="size-6 animate-spin text-muted-foreground" />
    </div>

    <template v-else>
      <!-- Sandbox Mode -->
      <SettingsItem :label="t('settings.workspace.sandboxMode')" :bordered="false">
        <div class="flex gap-2">
          <button
            class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-colors"
            :class="
              sandboxMode === 'codex'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'
            "
            @click="sandboxMode = 'codex'"
          >
            <ShieldCheck class="size-4" />
            {{ t('settings.workspace.modeCodex') }}
          </button>
          <button
            class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-colors"
            :class="
              sandboxMode === 'native'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'
            "
            @click="sandboxMode = 'native'"
          >
            <Monitor class="size-4" />
            {{ t('settings.workspace.modeNative') }}
          </button>
        </div>
      </SettingsItem>

      <!-- Sandbox Mode Description -->
      <div class="px-4 pb-4">
        <p class="text-xs text-muted-foreground">
          {{ modeDescription }}
        </p>
      </div>

      <!-- Sandbox Permissions (only when codex mode) -->
      <div
        v-if="sandboxMode === 'codex'"
        class="border-b border-border p-4 dark:border-white/10"
      >
        <span class="mb-3 block text-sm font-medium text-foreground">
          {{ t('settings.workspace.permissions') }}
        </span>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <Globe class="size-4 text-muted-foreground" />
            <span class="text-sm">{{ t('settings.workspace.networkAccess') }}</span>
          </div>
          <Switch
            :checked="sandboxNetwork"
            @update:checked="sandboxNetwork = $event"
          />
        </div>
        <p class="mt-2 text-xs text-muted-foreground">
          {{ t('settings.workspace.networkAccessDesc') }}
        </p>
      </div>

      <!-- Working Directory -->
      <SettingsItem :label="t('settings.workspace.workDir')">
        <div class="flex items-center gap-2">
          <span class="max-w-56 truncate text-sm text-muted-foreground" :title="displayWorkDir">
            {{ displayWorkDir }}
          </span>
          <Button
            v-if="appStore.isGUIMode"
            variant="outline"
            size="sm"
            class="shrink-0"
            @click="handleSelectDir"
          >
            <FolderOpen class="mr-1.5 size-3.5" />
            {{ t('settings.workspace.changeDir') }}
          </Button>
        </div>
      </SettingsItem>

      <!-- Working Directory Description -->
      <div class="px-4 pb-3">
        <p class="text-xs text-muted-foreground">
          {{ t('settings.workspace.workDirDesc') }}
        </p>
      </div>

      <!-- Save Button -->
      <div class="flex justify-end border-t border-border p-4 dark:border-white/10">
        <Button :disabled="saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="mr-2 size-4 animate-spin" />
          {{ t('common.save') }}
        </Button>
      </div>
    </template>
  </SettingsCard>
</template>
