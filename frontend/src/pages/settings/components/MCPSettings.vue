<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { FolderOpen } from 'lucide-vue-next'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'

const { t } = useI18n()

const mcpEnabled = ref(false)
const mcpDir = ref('')

const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategoryMCP)
    const enabledSetting = settings.find((s) => s.key === 'mcp_enabled')
    if (enabledSetting) {
      mcpEnabled.value = enabledSetting.value === 'true'
    }
  } catch (error) {
    console.error('Failed to load MCP settings:', error)
  }

  try {
    const dir = await SettingsService.GetMCPDir()
    mcpDir.value = dir
  } catch (error) {
    console.error('Failed to get MCP directory:', error)
  }
}

const handleMCPEnabledChange = async (val: boolean) => {
  const prev = mcpEnabled.value
  mcpEnabled.value = val
  try {
    await SettingsService.SetValue('mcp_enabled', String(val))
  } catch (error) {
    console.error('Failed to update mcp_enabled setting:', error)
    mcpEnabled.value = prev
  }
}

const handleOpenMCPDir = async () => {
  if (!mcpDir.value) return
  try {
    await BrowserService.OpenDirectory(mcpDir.value)
  } catch (error) {
    console.error('Failed to open MCP directory:', error)
  }
}

onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <SettingsCard :title="t('settings.mcp.title')">
      <SettingsItem>
        <template #label>
          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium text-foreground">{{ t('settings.mcp.enable') }}</span>
            <span class="text-xs text-muted-foreground">{{ t('settings.mcp.enableHint') }}</span>
          </div>
        </template>
        <Switch
          :model-value="mcpEnabled"
          @update:model-value="handleMCPEnabledChange"
        />
      </SettingsItem>

      <div class="flex flex-col gap-1 p-4">
        <span class="text-sm font-medium text-foreground">{{ t('settings.mcp.directory') }}</span>
        <span class="text-xs text-muted-foreground">{{ t('settings.mcp.directoryHint') }}</span>
        <div class="flex w-full items-center gap-2 pt-0.5">
          <Input
            :model-value="mcpDir"
            readonly
            class="flex-1 min-w-0 cursor-default bg-muted/30"
          />
          <button
            class="inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="handleOpenMCPDir"
          >
            <FolderOpen class="size-4" />
          </button>
        </div>
      </div>
    </SettingsCard>
  </div>
</template>
