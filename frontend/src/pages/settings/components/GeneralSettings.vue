<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AcceptableValue } from 'reka-ui'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAppStore, type Theme } from '@/stores'
import { useLocale } from '@/composables/useLocale'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

const { t } = useI18n()
const appStore = useAppStore()
const { locale: currentLocale, switchLocale } = useLocale()

// 语言选项
const languageOptions = [
  { value: 'zh-CN', label: 'settings.languages.zhCN' },
  { value: 'en-US', label: 'settings.languages.enUS' },
]

// 主题选项
const themeOptions = [
  { value: 'light', label: 'settings.themes.light' },
  { value: 'dark', label: 'settings.themes.dark' },
  { value: 'system', label: 'settings.themes.system' },
]

// 当前语言显示文本
const currentLanguageLabel = computed(() => {
  const option = languageOptions.find((opt) => opt.value === currentLocale.value)
  return option ? t(option.label) : ''
})

// 当前主题显示文本
const currentThemeLabel = computed(() => {
  const option = themeOptions.find((opt) => opt.value === appStore.theme)
  return option ? t(option.label) : ''
})

// 处理语言切换
const handleLanguageChange = (value: AcceptableValue) => {
  if (typeof value === 'string') {
    void switchLocale(value as 'zh-CN' | 'en-US')
  }
}

// 处理主题切换
const handleThemeChange = (value: AcceptableValue) => {
  if (typeof value === 'string') {
    appStore.setTheme(value as Theme)
  }
}
</script>

<template>
  <SettingsCard :title="t('settings.general.title')">
    <!-- 语言设置 -->
    <SettingsItem :label="t('settings.general.language')">
      <Select :model-value="currentLocale" @update:model-value="handleLanguageChange">
        <SelectTrigger class="w-80">
          <SelectValue>{{ currentLanguageLabel }}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="option in languageOptions"
            :key="option.value"
            :value="option.value"
          >
            {{ t(option.label) }}
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingsItem>

    <!-- 主题设置 -->
    <SettingsItem :label="t('settings.general.theme')" :bordered="false">
      <Select :model-value="appStore.theme" @update:model-value="handleThemeChange">
        <SelectTrigger class="w-80">
          <SelectValue>{{ currentThemeLabel }}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="option in themeOptions"
            :key="option.value"
            :value="option.value"
          >
            {{ t(option.label) }}
          </SelectItem>
        </SelectContent>
      </Select>
    </SettingsItem>
  </SettingsCard>
</template>
