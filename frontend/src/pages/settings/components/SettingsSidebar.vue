<script setup lang="ts">
import type { FunctionalComponent, SVGAttributes } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { useSettingsStore, type SettingsMenuItem } from '@/stores'

// 导入图标（作为 Vue 组件）
import ModelServiceIcon from '@/assets/icons/model-service.svg'
import GeneralSettingsIcon from '@/assets/icons/general-settings.svg'
import SnapSettingsIcon from '@/assets/icons/snap-settings.svg'
import ToolsIcon from '@/assets/icons/tools.svg'
import AboutIcon from '@/assets/icons/about.svg'

const { t } = useI18n()
const settingsStore = useSettingsStore()

interface MenuItem {
  id: SettingsMenuItem
  labelKey: string
  icon: FunctionalComponent<SVGAttributes>
}

const menuItems: MenuItem[] = [
  { id: 'modelService', labelKey: 'settings.menu.modelService', icon: ModelServiceIcon },
  { id: 'generalSettings', labelKey: 'settings.menu.generalSettings', icon: GeneralSettingsIcon },
  { id: 'snapSettings', labelKey: 'settings.menu.snapSettings', icon: SnapSettingsIcon },
  { id: 'tools', labelKey: 'settings.menu.tools', icon: ToolsIcon },
  { id: 'about', labelKey: 'settings.menu.about', icon: AboutIcon },
]

const handleMenuClick = (menuId: SettingsMenuItem) => {
  settingsStore.setActiveMenu(menuId)
}
</script>

<template>
  <nav
    class="flex h-full w-settings-sidebar flex-col gap-0.5 border-r border-border bg-background py-2 dark:border-white/10 dark:bg-background/50"
  >
    <button
      v-for="item in menuItems"
      :key="item.id"
      :class="
        cn(
          'group mx-2 flex h-9 items-center gap-2.5 rounded-md px-2.5 text-left text-sm transition-colors',
          settingsStore.activeMenu === item.id
            ? 'bg-accent font-medium text-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
        )
      "
      @click="handleMenuClick(item.id)"
    >
      <component
        :is="item.icon"
        class="size-4 transition-opacity"
        :class="
          settingsStore.activeMenu === item.id
            ? 'opacity-100'
            : 'opacity-70 group-hover:opacity-100'
        "
      />
      <span>{{ t(item.labelKey) }}</span>
    </button>
  </nav>
</template>
