<script setup lang="ts">
/**
 * 左侧导航菜单组件
 * 包含 AI助手、知识库、多问、设置 四个导航项
 * 点击导航项会切换/打开对应模块的标签页：
 * - AI助手：总是新建标签页
 * - 知识库、多问、设置：已有则切换，否则新建
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type SvgComponent = any
import { useI18n } from 'vue-i18n'
import { useNavigationStore, type NavModule } from '@/stores'
import { cn } from '@/lib/utils'
import IconAssistant from '@/assets/icons/assistant.svg'
import IconKnowledge from '@/assets/icons/knowledge.svg'
import IconMultiask from '@/assets/icons/multiask.svg'
import IconSettings from '@/assets/icons/settings.svg'

const { t } = useI18n()
const navigationStore = useNavigationStore()

/**
 * 导航项配置
 */
interface NavItem {
  key: NavModule
  labelKey: string
  icon: SvgComponent
}

/**
 * 顶部导航项（AI助手、知识库、多问）
 */
const topNavItems: NavItem[] = [
  {
    key: 'assistant',
    labelKey: 'nav.assistant',
    icon: IconAssistant,
  },
  {
    key: 'knowledge',
    labelKey: 'nav.knowledge',
    icon: IconKnowledge,
  },
  {
    key: 'multiask',
    labelKey: 'nav.multiask',
    icon: IconMultiask,
  },
]

/**
 * 底部导航项（设置）
 */
const bottomNavItems: NavItem[] = [
  {
    key: 'settings',
    labelKey: 'nav.settings',
    icon: IconSettings,
  },
]

/**
 * 处理导航项点击
 * 点击时自动创建对应模块的新标签页
 */
const handleNavClick = (module: NavModule) => {
  navigationStore.navigateToModule(module)
}
</script>

<template>
  <div
    :class="
      cn(
        'flex shrink-0 flex-col items-center justify-between overflow-hidden border-r border-solid border-border bg-background py-3 transition-all duration-200',
        navigationStore.sidebarCollapsed ? 'w-13' : 'w-44'
      )
    "
  >
    <!-- 顶部导航区域 -->
    <div class="flex w-full flex-col gap-1">
      <button
        v-for="item in topNavItems"
        :key="item.key"
        :class="
          cn(
            'group mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors',
            navigationStore.sidebarCollapsed && 'justify-center',
            navigationStore.activeModule === item.key
              ? 'bg-accent text-accent-foreground font-medium'
              : 'text-muted-foreground hover:bg-accent/50 hover:text-accent-foreground'
          )
        "
        :title="navigationStore.sidebarCollapsed ? t(item.labelKey) : undefined"
        @click="handleNavClick(item.key)"
      >
        <component
          :is="item.icon"
          :class="
            cn(
              'size-4 shrink-0 transition-opacity',
              navigationStore.activeModule === item.key
                ? 'opacity-100'
                : 'opacity-70 group-hover:opacity-100'
            )
          "
        />
        <span v-if="!navigationStore.sidebarCollapsed">{{ t(item.labelKey) }}</span>
      </button>
    </div>

    <!-- 底部导航区域 -->
    <div class="flex w-full flex-col gap-1">
      <button
        v-for="item in bottomNavItems"
        :key="item.key"
        :class="
          cn(
            'group mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors',
            navigationStore.sidebarCollapsed && 'justify-center',
            navigationStore.activeModule === item.key
              ? 'bg-accent text-accent-foreground font-medium'
              : 'text-muted-foreground hover:bg-accent/50 hover:text-accent-foreground'
          )
        "
        :title="navigationStore.sidebarCollapsed ? t(item.labelKey) : undefined"
        @click="handleNavClick(item.key)"
      >
        <component
          :is="item.icon"
          :class="
            cn(
              'size-4 shrink-0 transition-opacity',
              navigationStore.activeModule === item.key
                ? 'opacity-100'
                : 'opacity-70 group-hover:opacity-100'
            )
          "
        />
        <span v-if="!navigationStore.sidebarCollapsed">{{ t(item.labelKey) }}</span>
      </button>
    </div>
  </div>
</template>
