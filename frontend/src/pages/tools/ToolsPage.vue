<script setup lang="ts">
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { MessageCircle, PanelLeft, Search, Send, Settings2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { useAppStore, useNavigationStore, useSettingsStore, type SystemOwner } from '@/stores'
import { useToolsGuiSettingsStore } from '@/stores/toolsGuiSettings'

const props = defineProps<{
  tabId: string
  systemOwner?: SystemOwner
}>()

const { t } = useI18n()
const appStore = useAppStore()
const navigationStore = useNavigationStore()
const settingsStore = useSettingsStore()
const toolsGuiStore = useToolsGuiSettingsStore()
const { enableSelectionSearch, showFloatingWindow } = storeToRefs(toolsGuiStore)

const owner = () => props.systemOwner ?? appStore.currentSystem

function openSnapSettings() {
  if (!appStore.isGUIMode) return
  settingsStore.setActiveMenu('snapSettings')
  navigationStore.navigateToModule('settings', owner())
}

function openToolsSettings() {
  if (!appStore.isGUIMode) return
  settingsStore.setActiveMenu('tools')
  navigationStore.navigateToModule('settings', owner())
}

function goMultiask() {
  navigationStore.navigateToModule('multiask', owner())
}

function toggleMultiaskInSidebar() {
  appStore.setShowMultiaskInNav(!appStore.showMultiaskInNav)
}

onMounted(() => {
  void toolsGuiStore.reloadFromBackend()
})

const cardClass = (interactive: boolean) =>
  cn(
    'flex flex-col rounded-xl border border-border bg-card p-4 text-left shadow-sm transition dark:shadow-none dark:ring-1 dark:ring-white/10',
    interactive && 'hover:bg-accent/30',
    !interactive && 'opacity-80'
  )
</script>

<template>
  <div class="flex h-full min-h-0 w-full flex-col overflow-y-auto bg-white dark:bg-background">
    <!-- Page Header -->
    <div class="flex h-20 shrink-0 items-center justify-between px-6">
      <div class="flex flex-col gap-1">
        <h1 class="text-base font-semibold text-[#262626] dark:text-foreground">
          {{ t('nav.tools') }}
        </h1>
        <p class="text-sm text-[#737373] dark:text-muted-foreground">
          {{ t('toolsPage.subtitle') }}
        </p>
      </div>
    </div>

    <div class="flex min-h-0 flex-1 flex-col overflow-auto px-6 pb-6">
      <div class="mx-auto w-full max-w-4xl">
        <p
          v-if="!appStore.isGUIMode"
          class="mt-4 rounded-lg border border-dashed border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground"
        >
          {{ t('toolsPage.desktopOnlyHint') }}
        </p>

        <div class="mt-8 grid gap-4 sm:grid-cols-2">
          <!-- Smart sidebar -->
          <div
            role="button"
            tabindex="0"
            :class="cardClass(appStore.isGUIMode)"
            @click="openSnapSettings"
            @keydown.enter="openSnapSettings"
          >
            <div class="flex gap-3">
              <div
                class="flex size-11 shrink-0 items-center justify-center rounded-lg bg-blue-500/15 text-blue-600 dark:text-blue-400"
              >
                <PanelLeft class="size-6" stroke-width="1.75" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="font-semibold text-foreground">
                  {{ t('toolsPage.smartSidebar.title') }}
                </div>
                <p class="mt-1 text-sm text-muted-foreground">
                  {{ t('toolsPage.smartSidebar.description') }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex justify-end">
              <Button
                type="button"
                variant="outline"
                size="sm"
                class="gap-1.5"
                :disabled="!appStore.isGUIMode"
                @click.stop="openSnapSettings"
              >
                <Settings2 class="size-4" stroke-width="1.75" />
                {{ t('nav.settings') }}
              </Button>
            </div>
          </div>

          <!-- Selection search -->
          <div
            role="button"
            tabindex="0"
            :class="cardClass(appStore.isGUIMode)"
            @click="openToolsSettings"
            @keydown.enter="openToolsSettings"
          >
            <div class="flex gap-3">
              <div
                class="flex size-11 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
              >
                <Search class="size-6" stroke-width="1.75" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="font-semibold text-foreground">
                  {{ t('toolsPage.selectionSearch.title') }}
                </div>
                <p class="mt-1 text-sm text-muted-foreground">
                  {{ t('toolsPage.selectionSearch.description') }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex justify-end">
              <div @click.stop>
                <Switch
                  :disabled="!appStore.isGUIMode"
                  :model-value="enableSelectionSearch"
                  @update:model-value="toolsGuiStore.setSelectionSearch"
                />
              </div>
            </div>
          </div>

          <!-- Floating icon -->
          <div
            role="button"
            tabindex="0"
            :class="cardClass(appStore.isGUIMode)"
            @click="openToolsSettings"
            @keydown.enter="openToolsSettings"
          >
            <div class="flex gap-3">
              <div
                class="flex size-11 shrink-0 items-center justify-center rounded-lg bg-violet-500/15 text-violet-600 dark:text-violet-400"
              >
                <Send class="size-6" stroke-width="1.75" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="font-semibold text-foreground">
                  {{ t('toolsPage.floatingIcon.title') }}
                </div>
                <p class="mt-1 text-sm text-muted-foreground">
                  {{ t('toolsPage.floatingIcon.description') }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex justify-end">
              <div @click.stop @click.capture="toolsGuiStore.handleFloatingWindowClickCapture">
                <Switch v-model="showFloatingWindow" :disabled="!appStore.isGUIMode" />
              </div>
            </div>
          </div>

          <!-- Multiask -->
          <div
            role="button"
            tabindex="0"
            :class="cardClass(true)"
            @click="goMultiask"
            @keydown.enter="goMultiask"
          >
            <div class="flex gap-3">
              <div
                class="flex size-11 shrink-0 items-center justify-center rounded-lg bg-rose-500/15 text-rose-600 dark:text-rose-400"
              >
                <MessageCircle class="size-6" stroke-width="1.75" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="font-semibold text-foreground">
                  {{ t('toolsPage.multiask.title') }}
                </div>
                <p class="mt-1 text-sm text-muted-foreground">
                  {{ t('toolsPage.multiask.description') }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex justify-end">
              <Button
                type="button"
                variant="outline"
                size="sm"
                @click.stop="toggleMultiaskInSidebar"
              >
                {{
                  appStore.showMultiaskInNav
                    ? t('toolsPage.removeFromMenuBar')
                    : t('toolsPage.addToMenuBar')
                }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
