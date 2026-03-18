<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { ChatWikiService, type Binding, type ModelCatalog } from '@bindings/chatclaw/internal/services/chatwiki'
import { getBinding as getChatwikiBinding } from '@/lib/chatwikiCache'
import { onChatwikiBindingChanged } from '@/lib/chatwikiBindingState'
import { useNavigationStore } from '@/stores'
import { useSettingsStore } from '@/stores/settings'
import {
  buildChatwikiSidebarAccountCardState,
  getChatwikiCreditsRefreshMode,
  isChatwikiCloudBinding,
  shouldAutoRefreshChatwikiCredits,
} from './chatwikiSidebarAccountCard'

const settingsStore = useSettingsStore()
const navigationStore = useNavigationStore()

const binding = ref<Binding | null>(null)
const modelCatalog = ref<ModelCatalog | null>(null)

const cardState = computed(() =>
  buildChatwikiSidebarAccountCardState(binding.value, modelCatalog.value)
)

let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  if (!shouldAutoRefreshChatwikiCredits(binding.value)) return
  autoRefreshTimer = setInterval(() => {
    void loadState('polling')
  }, 10_000)
}

async function loadState(mode: 'initial' | 'polling' = 'initial') {
  binding.value = await getChatwikiBinding().catch(() => null)

  if (!isChatwikiCloudBinding(binding.value)) {
    modelCatalog.value = null
    stopAutoRefresh()
    return
  }

  modelCatalog.value =
    (await ChatWikiService.GetModelCatalog(getChatwikiCreditsRefreshMode(mode)).catch(() => null)) ??
    null
  startAutoRefresh()
}

function handleClick() {
  if (cardState.value.action === 'login') {
    settingsStore.requestChatwikiCloudLogin()
    settingsStore.setActiveMenu('chatwiki')
    navigationStore.navigateToModule('settings')
    return
  }

  settingsStore.requestModelServiceProviderSelection('chatwiki')
  settingsStore.setActiveMenu('modelService')
  navigationStore.navigateToModule('settings')
}

let unsubscribeBindingChanged: (() => void) | null = null
let unsubscribeModelsChanged: (() => void) | null = null

onMounted(() => {
  void loadState()
  unsubscribeBindingChanged = onChatwikiBindingChanged(() => {
    void loadState()
  })
  unsubscribeModelsChanged = Events.On('models:changed', () => {
    void loadState()
  })
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  unsubscribeBindingChanged?.()
  unsubscribeModelsChanged?.()
})
</script>

<template>
  <div id="sidebar-account-status" class="w-full px-2">
    <div
      class="flex flex-col overflow-hidden rounded-lg border border-blue-100/50 bg-blue-50/50 px-3 py-2 text-gray-500 transition-colors hover:bg-blue-50"
      role="button"
      tabindex="0"
      @click="handleClick"
      @keydown.enter.prevent="handleClick"
      @keydown.space.prevent="handleClick"
    >
      <span v-if="cardState.mode === 'bound'" class="w-full truncate text-[11px] text-gray-400">
        {{ cardState.accountLabel }}
      </span>
      <span class="text-[13px] font-bold text-blue-600">
        {{ cardState.creditsLabel }}
      </span>
    </div>
  </div>
</template>
