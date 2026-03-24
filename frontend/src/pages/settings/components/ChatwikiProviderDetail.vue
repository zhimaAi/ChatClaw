<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import {
  ChevronDown,
  ChevronUp,
  File,
  FileText,
  Image as ImageIcon,
  LoaderCircle,
  Mic,
  RefreshCw,
  UserCheck,
  Video,
} from 'lucide-vue-next'
import ModelIcon from '@/assets/icons/model.svg'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useSettingsStore } from '@/stores/settings'
import type { Provider, ProviderWithModels } from '@bindings/chatclaw/internal/services/providers'
import { ProvidersService, UpdateProviderInput } from '@bindings/chatclaw/internal/services/providers'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import {
  ChatWikiService,
  type Binding,
  type ModelCatalog,
  type ModelCatalogItem,
} from '@bindings/chatclaw/internal/services/chatwiki'
import {
  shouldShowChatwikiAccountCard,
  shouldShowChatwikiCreditsCard,
} from './chatwikiProviderDetailState'

const props = defineProps<{
  providerWithModels: ProviderWithModels
  loading?: boolean
  errorMessage?: string | null
}>()

const emit = defineEmits<{
  update: [provider: Provider]
}>()

type CatalogGroup = {
  type: string
  title: string
  models: ModelCatalogItem[]
}

type ChatwikiModelCatalogItem = ModelCatalogItem & {
  price?: string
}

const { t } = useI18n()
const settingsStore = useSettingsStore()

const localEnabled = ref(false)
const loadingBinding = ref(false)
const loadingCatalog = ref(false)
const hasLoadedCatalogOnce = ref(false)
const savingToggle = ref(false)
const openingBilling = ref(false)
const currentBinding = ref<Binding | null>(null)
const modelCatalog = ref<ModelCatalog | null>(null)
const cloudURL = ref('')
const catalogError = ref('')
const llmRegionFilter = ref<'all' | 'CN' | 'Global'>('all')
const collapsedGroups = ref<Record<string, boolean>>({})
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
let unsubscribeCatalogRefresh: (() => void) | null = null

const isBound = computed(() => !!currentBinding.value && modelCatalog.value?.bound !== false)
const showAccountCard = computed(() => {
  return isBound.value && shouldShowChatwikiAccountCard(currentBinding.value)
})
const showCreditsCard = computed(() => {
  return showAccountCard.value && shouldShowChatwikiCreditsCard(currentBinding.value)
})
const showDevLoginButton = computed(() => {
  return (
    showAccountCard.value &&
    !showCreditsCard.value &&
    currentBinding.value?.chatwiki_version?.trim().toLowerCase() === 'dev'
  )
})
const todayUse = computed(() => extractStatValue(modelCatalog.value, 'today_use'))
const allSurplus = computed(() => extractStatValue(modelCatalog.value, 'all_surplus'))
const bindingDisplayName = computed(() => {
  return currentBinding.value?.user_name?.trim() || currentBinding.value?.user_id?.trim() || 'ChatWiki'
})

const modelGroups = computed<CatalogGroup[]>(() => {
  const catalog = modelCatalog.value
  if (!catalog) return []

  return [
    {
      type: 'llm',
      title: t('settings.modelService.llmModels'),
      models: catalog.llm_models || [],
    },
    {
      type: 'embedding',
      title: t('settings.modelService.embeddingModels'),
      models: catalog.embedding_models || [],
    },
    {
      type: 'rerank',
      title: t('settings.modelService.rerankModels'),
      models: catalog.rerank_models || [],
    },
  ].filter((group) => group.models.length > 0)
})

const capabilityIcons: Record<string, any> = {
  text: FileText,
  image: ImageIcon,
  audio: Mic,
  video: Video,
  file: File,
}

function toStatText(value: string): string {
  if (!value) return '--'
  const num = Number(value)
  if (Number.isNaN(num)) return value
  return num.toLocaleString(undefined, { maximumFractionDigits: 3 })
}

function extractStatValue(catalog: ModelCatalog | null, key: string): string {
  const raw = catalog?.integral_stats?.raw
  if (!raw) return ''

  try {
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
    return findDeepStringValue(parsed, key)
  } catch {
    return ''
  }
}

function findDeepStringValue(input: unknown, key: string): string {
  if (!input || typeof input !== 'object') return ''

  if (Array.isArray(input)) {
    for (const item of input) {
      const nested = findDeepStringValue(item, key)
      if (nested) return nested
    }
    return ''
  }

  const record = input as Record<string, unknown>
  if (record[key] != null) {
    return String(record[key])
  }

  for (const value of Object.values(record)) {
    const nested = findDeepStringValue(value, key)
    if (nested) return nested
  }

  return ''
}

function normalizeText(value?: string | null): string {
  return value?.trim() || ''
}

function getModelPrimaryName(model: ModelCatalogItem): string {
  const supplier = normalizeText(model.model_supplier)
  const uniModelName = normalizeText(model.uni_model_name)
  if (supplier && uniModelName) {
    return `${supplier}/${uniModelName}`
  }
  if (uniModelName) {
    return uniModelName
  }
  return normalizeText(model.name) || normalizeText(model.model_id) || '-'
}

function getModelPrice(model: ModelCatalogItem): string {
  const price = normalizeText((model as ChatwikiModelCatalogItem).price)
  if (!price) return ''
  return t('settings.chatwiki.pricePerKToken', { price })
}

function getModelMetaText(model: ModelCatalogItem): string {
  return getModelPrice(model)
}

function getRegionScopeLabel(regionScope?: string): string {
  switch (normalizeText(regionScope)) {
    case 'CN':
      return t('settings.chatwiki.region.cn')
    case 'Global':
      return t('settings.chatwiki.region.global')
    default:
      return ''
  }
}

function getVisibleModels(group: CatalogGroup): ModelCatalogItem[] {
  if (group.type !== 'llm' || llmRegionFilter.value === 'all') {
    return group.models
  }
  return group.models.filter((model) => normalizeText(model.region_scope) === llmRegionFilter.value)
}

function isGroupCollapsed(groupType: string): boolean {
  return collapsedGroups.value[groupType] === true
}

function toggleGroup(groupType: string) {
  collapsedGroups.value = {
    ...collapsedGroups.value,
    [groupType]: !collapsedGroups.value[groupType],
  }
}

async function loadBinding() {
  loadingBinding.value = true
  try {
    currentBinding.value = (await ChatWikiService.GetBinding()) ?? null
  } catch (error) {
    console.error('Failed to load ChatWiki binding:', error)
    currentBinding.value = null
  } finally {
    loadingBinding.value = false
  }
}

async function loadCatalog(forceRefresh = false, silent = false) {
  const shouldShowLoading = !silent && (!hasLoadedCatalogOnce.value || !modelCatalog.value)
  if (shouldShowLoading) {
    loadingCatalog.value = true
  }
  catalogError.value = ''

  try {
    modelCatalog.value = forceRefresh
      ? ((await ChatWikiService.GetModelCatalog(true)) ?? null)
      : ((await ChatWikiService.GetModelCatalog(false)) ?? null)
    hasLoadedCatalogOnce.value = true
  } catch (error) {
    console.error('Failed to load ChatWiki model catalog:', error)
    catalogError.value = getErrorMessage(error) || t('settings.chatwiki.modelLoadFailed')
    modelCatalog.value = null
  } finally {
    if (shouldShowLoading) {
      loadingCatalog.value = false
    }
  }
}

async function loadPageData(forceRefresh = false) {
  await Promise.all([loadBinding(), loadCatalog(forceRefresh)])
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshTimer = setInterval(() => {
    if (!shouldShowChatwikiCreditsCard(currentBinding.value)) return
    void loadCatalog(true, true)
  }, 10_000)
}

async function handleToggle(checked: boolean | 'indeterminate') {
  const enabled = checked === true
  const previous = localEnabled.value
  localEnabled.value = enabled
  savingToggle.value = true
  try {
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ enabled })
    )
    if (updated) {
      emit('update', updated)
    }
  } catch (error) {
    localEnabled.value = previous
    toast.error(getErrorMessage(error))
  } finally {
    savingToggle.value = false
  }
}

function goToBindingSettings() {
  settingsStore.setActiveMenu('chatwiki')
}

async function handleLoginNow() {
  settingsStore.requestChatwikiCloudLogin()
  goToBindingSettings()
}

async function openBillingPage() {
  const base = cloudURL.value.trim().replace(/\/+$/, '')
  if (!base) {
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }

  openingBilling.value = true
  try {
    await BrowserService.OpenURL(`${base}/#/user/model`)
  } catch (error) {
    console.error('Failed to open ChatWiki billing page:', error)
    toast.error(getErrorMessage(error) || t('settings.chatwiki.openBillingFailed'))
  } finally {
    openingBilling.value = false
  }
}

watch(
  () => props.providerWithModels.provider,
  (provider) => {
    localEnabled.value = provider.enabled
    void loadPageData(true)
    void ChatWikiService.GetCloudURL().then((url) => {
      cloudURL.value = url ?? ''
    })
    startAutoRefresh()
  },
  { immediate: true }
)

unsubscribeCatalogRefresh = Events.On('chatwiki:model-catalog-refresh', () => {
  void loadPageData(true)
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  unsubscribeCatalogRefresh?.()
})
</script>

<template>
  <div class="mx-auto w-full max-w-settings-card">
    <div class="flex flex-col gap-6">
      <div
        class="flex items-center justify-between bg-background px-4 py-4"
      >
        <div>
          <p class="text-base font-semibold text-foreground">
            {{ providerWithModels.provider.name }}
          </p>
        </div>
        <Switch
          :model-value="localEnabled"
          :disabled="savingToggle"
          @update:model-value="handleToggle"
        />
      </div>

      <div
        v-if="props.errorMessage || catalogError"
        class="rounded-xl border border-border bg-muted/20 p-3 text-sm text-muted-foreground"
      >
        {{ props.errorMessage || catalogError }}
      </div>

      <div class="flex flex-col gap-6">
        <div
          v-if="showAccountCard"
          class="rounded-[28px] bg-card p-6 shadow-sm dark:shadow-none"
        >
          <div class="flex items-start justify-between gap-4">
            <div class="flex min-w-0 items-center gap-4">
              <div
                class="flex size-12 shrink-0 items-center justify-center rounded-full bg-blue-50 text-blue-500 dark:bg-blue-500/10 dark:text-blue-400"
              >
                <UserCheck class="size-5 stroke-[1.8]" />
              </div>
              <div class="min-w-0">
                <p class="truncate text-base font-medium text-foreground">
                  {{ bindingDisplayName }}
                </p>
              </div>
            </div>

            <Button
              v-if="showCreditsCard"
              size="sm"
              class="h-9 rounded-xl bg-slate-950 px-4 text-sm font-medium text-white hover:bg-slate-800 dark:bg-white dark:text-slate-950 dark:hover:bg-white/90"
              :disabled="openingBilling"
              @click="openBillingPage"
            >
              {{ t('settings.chatwiki.buyCredits') }}
            </Button>
            <Button
              v-else-if="showDevLoginButton"
              size="sm"
              class="h-9 rounded-lg bg-[#2f67f6] px-3.5 text-sm font-medium text-white shadow-[0_4px_10px_rgba(47,103,246,0.2)] hover:bg-[#2558db]"
              @click="handleLoginNow"
            >
              {{ t('settings.chatwiki.loginNow') }}
            </Button>
          </div>

          <div v-if="showCreditsCard" class="mt-6 grid gap-4 md:grid-cols-2">
            <div
              class="bg-background/80 px-6 py-5"
            >
              <p class="text-sm text-muted-foreground">{{ t('settings.chatwiki.todayUse') }}</p>
              <div class="mt-4 flex items-baseline gap-2">
                <span class="text-[2rem] font-semibold leading-none tracking-tight text-orange-600">
                  {{ toStatText(todayUse) }}
                </span>
                <span class="text-sm font-medium text-muted-foreground">
                  {{ t('settings.chatwiki.pointsUnit') }}
                </span>
              </div>
            </div>
            <div
              class="bg-background/80 px-6 py-5"
            >
              <p class="text-sm text-muted-foreground">
                {{ t('settings.chatwiki.remainingCredits') }}
              </p>
              <div class="mt-4 flex items-baseline gap-2">
                <span class="text-[2rem] font-semibold leading-none tracking-tight text-primary">
                  {{ toStatText(allSurplus) }}
                </span>
                <span class="text-sm font-medium text-muted-foreground">
                  {{ t('settings.chatwiki.pointsUnit') }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          v-else-if="!isBound"
          class="rounded-[28px] border border-border/60 bg-card p-8 shadow-[0_10px_30px_rgba(15,23,42,0.06)] dark:border-white/10 dark:shadow-none"
        >
          <div class="flex items-center justify-between gap-6">
            <div class="flex min-w-0 items-center gap-5">
              <div
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-blue-50"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                  id="account-icon"
                  class="h-5 w-5 text-blue-500"
                >
                  <path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"></path>
                  <circle cx="12" cy="7" r="4"></circle>
                </svg>
              </div>
              <div class="min-w-0">
                <p class="text-xl font-semibold leading-tight text-foreground">
                  {{ t('settings.chatwiki.notLoggedInTitle') }}
                </p>
              </div>
            </div>

            <Button
              size="sm"
              class="h-9 rounded-lg bg-[#2f67f6] px-3.5 text-sm font-medium text-white shadow-[0_4px_10px_rgba(47,103,246,0.2)] hover:bg-[#2558db]"
              @click="handleLoginNow"
            >
              {{ t('settings.chatwiki.loginNow') }}
            </Button>
          </div>
        </div>

        <div
          v-if="loadingBinding || loadingCatalog"
          class="flex items-center justify-center px-4 py-12 text-sm text-muted-foreground"
        >
          <LoaderCircle class="mr-2 size-4 animate-spin" />
          {{ t('settings.modelService.loadingProviders') }}
        </div>

        <div
          v-if="!loadingBinding && !loadingCatalog && modelGroups.length === 0"
          class="px-5 py-10 text-center text-sm text-muted-foreground"
        >
          {{ t('settings.modelService.noModels') }}
        </div>

        <div v-else-if="!loadingBinding && !loadingCatalog" class="flex flex-col gap-4">
          <div
            v-for="group in modelGroups"
            :key="group.type"
            class="overflow-hidden rounded-xl border border-border dark:border-white/10"
          >
            <div
              class="flex items-center justify-between border-b border-border bg-background px-4 py-3 dark:border-white/10"
            >
              <h4 class="text-sm font-medium text-foreground">
                {{ group.title }}
              </h4>
              <div class="flex items-center gap-3">
                <select
                  v-if="group.type === 'llm'"
                  v-model="llmRegionFilter"
                  class="h-9 rounded-xl border border-border bg-background px-3 text-sm text-foreground outline-none transition-colors hover:border-border/80 dark:border-white/10"
                >
                  <option value="all">{{ t('settings.chatwiki.region.all') }}</option>
                  <option value="CN">{{ t('settings.chatwiki.region.cn') }}</option>
                  <option value="Global">{{ t('settings.chatwiki.region.global') }}</option>
                </select>
                <button
                  type="button"
                  class="text-muted-foreground transition-colors hover:text-foreground"
                  @click="toggleGroup(group.type)"
                >
                  <ChevronDown v-if="isGroupCollapsed(group.type)" class="size-4" />
                  <ChevronUp v-else class="size-4" />
                </button>
              </div>
            </div>

            <div v-if="!isGroupCollapsed(group.type)" class="p-1.5">
              <div
                v-for="model in getVisibleModels(group)"
                :key="`${group.type}-${model.model_id}`"
                class="flex items-center gap-3 rounded-lg px-3 py-3 hover:bg-accent/40"
              >
                <ModelIcon class="size-5 shrink-0 text-muted-foreground" />
                <div class="min-w-0 flex flex-1 items-center gap-2 overflow-hidden">
                  <p class="truncate whitespace-nowrap text-sm font-normal text-foreground no-underline">
                    {{ getModelPrimaryName(model) }}
                  </p>
                  <div
                    v-if="getModelMetaText(model) || getRegionScopeLabel(model.region_scope)"
                    class="flex min-w-0 items-center gap-2 overflow-hidden whitespace-nowrap text-xs text-muted-foreground"
                  >
                    <span
                      v-if="getModelMetaText(model)"
                      class="truncate whitespace-nowrap"
                    >
                      {{ getModelMetaText(model) }}
                    </span>
                    <span
                      v-if="getRegionScopeLabel(model.region_scope)"
                      class="shrink-0 rounded-md bg-muted px-2 py-0.5 text-[11px] text-muted-foreground"
                    >
                      {{ getRegionScopeLabel(model.region_scope) }}
                    </span>
                  </div>
                </div>
                <div
                  v-if="group.type !== 'embedding'"
                  class="flex shrink-0 items-center gap-2 text-muted-foreground"
                >
                  <component
                    :is="capabilityIcons[cap]"
                    v-for="cap in model.capabilities"
                    :key="cap"
                    class="size-3.5"
                  />
                </div>
              </div>
              <div
                v-if="getVisibleModels(group).length === 0"
                class="px-3 py-6 text-center text-sm text-muted-foreground"
              >
                {{ t('settings.modelService.noModels') }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
