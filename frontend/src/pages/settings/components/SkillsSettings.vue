<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { FolderOpen } from 'lucide-vue-next'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { SkillsService } from '@bindings/chatclaw/internal/services/skills'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'

const { t } = useI18n()

const skillsEnabled = ref(true)
const skillsDir = ref('')

const loadSettings = async () => {
  try {
    const settings = await SettingsService.List(Category.CategorySkills)
    const enabledSetting = settings.find((s) => s.key === 'skills_enabled')
    if (enabledSetting) {
      skillsEnabled.value = enabledSetting.value === 'true'
    }
  } catch (error) {
    console.error('Failed to load skills settings:', error)
  }

  try {
    const dir = await SkillsService.GetSkillsDir()
    skillsDir.value = dir
  } catch (error) {
    console.error('Failed to get skills directory:', error)
  }
}

const handleSkillsEnabledChange = async (val: boolean) => {
  const prev = skillsEnabled.value
  skillsEnabled.value = val
  try {
    await SettingsService.SetValue('skills_enabled', String(val))
  } catch (error) {
    console.error('Failed to update skills_enabled setting:', error)
    skillsEnabled.value = prev
  }
}

const handleOpenSkillsDir = async () => {
  if (!skillsDir.value) return
  try {
    await BrowserService.OpenDirectory(skillsDir.value)
  } catch (error) {
    console.error('Failed to open skills directory:', error)
  }
}

onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <SettingsCard :title="t('settings.skills.title')">
      <SettingsItem>
        <template #label>
          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium text-foreground">{{ t('settings.skills.enable') }}</span>
            <span class="text-xs text-muted-foreground">{{ t('settings.skills.enableHint') }}</span>
          </div>
        </template>
        <Switch
          :model-value="skillsEnabled"
          @update:model-value="handleSkillsEnabledChange"
        />
      </SettingsItem>

      <div
        class="flex flex-col gap-1 p-4"
      >
        <span class="text-sm font-medium text-foreground">{{ t('settings.skills.directory') }}</span>
        <span class="text-xs text-muted-foreground">{{ t('settings.skills.directoryHint') }}</span>
        <div class="flex w-full items-center gap-2 pt-0.5">
          <Input
            :model-value="skillsDir"
            readonly
            class="flex-1 min-w-0 cursor-default bg-muted/30"
          />
          <button
            class="inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="handleOpenSkillsDir"
          >
            <FolderOpen class="size-4" />
          </button>
        </div>
      </div>
    </SettingsCard>
  </div>
</template>
