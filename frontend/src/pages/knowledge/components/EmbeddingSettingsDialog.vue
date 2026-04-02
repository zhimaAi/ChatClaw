<script setup lang="ts">
import { computed, nextTick, ref, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useNavigationStore, useSettingsStore } from '@/stores'
import FieldLabel from './FieldLabel.vue'
import OrangeWarning from './OrangeWarning.vue'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type {
  Provider,
  ProviderWithModels,
  Model,
} from '@bindings/chatclaw/internal/services/providers'
import { ProvidersService } from '@bindings/chatclaw/internal/services/providers'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import { getBinding as getChatwikiBinding } from '@/lib/chatwikiCache'
import { onChatwikiBindingChanged } from '@/lib/chatwikiBindingState'
import {
  clearUnavailableChatwikiSelection,
  formatModelDisplayLabel,
  formatProviderDisplayLabel,
  getChatwikiAvailabilityStatus,
  getFirstSelectableModelKey,
  isModelSelectionDisabled,
  isSelectionAvailable,
} from '@/lib/chatwikiModelAvailability'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t } = useI18n()
const navigationStore = useNavigationStore()
const settingsStore = useSettingsStore()

const loading = ref(false)
const saving = ref(false)
const embeddingSelectOpen = ref(false)
const chatwikiAvailability = ref<'available' | 'unbound' | 'non_cloud'>('available')
let unsubscribeChatwikiBindingChanged: (() => void) | null = null

type Group = { provider: Provider; models: Model[] }
const embeddingGroups = ref<Group[]>([])

type ChatwikiDisplayModel = Model & {
  model_supplier?: string
  uni_model_name?: string
}

const normalizeText = (value?: string | null) => value?.trim() || ''

const DEFAULT_EMBEDDING_DIMENSION = '1536'
const TEXT_EMBEDDING_V3_DIMENSION = '1024'

const getDefaultEmbeddingDimension = (selectedKey: string) => {
  const [, modelId] = selectedKey.split('::')
  return modelId === 'text-embedding-v3'
    ? TEXT_EMBEDDING_V3_DIMENSION
    : DEFAULT_EMBEDDING_DIMENSION
}

const getEmbeddingModelLabel = (providerId: string, model: Model) => {
  let label = normalizeText(model.name) || normalizeText(model.model_id) || '-'
  if (providerId === 'chatwiki') {
    const chatwikiModel = model as ChatwikiDisplayModel
    const supplier = normalizeText(chatwikiModel.model_supplier)
    const uniModelName = normalizeText(chatwikiModel.uni_model_name)
    if (supplier && uniModelName) label = `${supplier}/${uniModelName}`
    else if (uniModelName) label = uniModelName
  }
  return formatModelDisplayLabel(providerId, label, chatwikiAvailability.value)
}

const embeddingSelectedKey = ref<string>('') // `${providerId}::${modelId}`
const embeddingDimension = ref<string>(DEFAULT_EMBEDDING_DIMENSION)

const embeddingCurrentLabel = computed(() => {
  const [pid, mid] = embeddingSelectedKey.value.split('::')
  if (!pid || !mid) return ''
  const provider = embeddingGroups.value.find((g) => g.provider.provider_id === pid)
  const model = provider?.models.find((m) => m.model_id === mid)
  return model ? getEmbeddingModelLabel(pid, model) : ''
})

function isProviderFree(g: Group): boolean {
  const p = g.provider as { is_free?: boolean }
  return Boolean(p?.is_free)
}

const selectedProviderIsFree = computed(() => {
  const [pid] = embeddingSelectedKey.value.split('::')
  if (!pid) return false
  const g = embeddingGroups.value.find((gr) => gr.provider.provider_id === pid)
  return g ? isProviderFree(g) : false
})

const close = () => emit('update:open', false)

async function goToChatwikiLogin() {
  embeddingSelectOpen.value = false
  close()
  await nextTick()
  settingsStore.requestChatwikiCloudLogin()
  settingsStore.setActiveMenu('chatwiki')
  navigationStore.navigateToModule('settings')
}

const isEmbeddingSelectionAvailable = computed(() => {
  return isSelectionAvailable(
    embeddingGroups.value.map((group) => ({
      provider: group.provider,
      model_groups: [{ type: 'embedding', models: group.models }],
    })),
    embeddingSelectedKey.value,
    'embedding',
    chatwikiAvailability.value
  )
})

const loadGroups = async () => {
  loading.value = true
  try {
    const [providers, binding] = await Promise.all([
      ProvidersService.ListProviders(),
      getChatwikiBinding().catch(() => null),
    ])
    chatwikiAvailability.value = getChatwikiAvailabilityStatus(binding)
    // 只使用"已启用"的供应商（已启动）
    const enabledProviders = providers.filter((p) => p.enabled)
    const details = await Promise.all(
      enabledProviders.map(async (p) => {
        try {
          const detail = await ProvidersService.GetProviderWithModels(p.provider_id)
          return { provider: p, detail }
        } catch (error: unknown) {
          // 单个 provider 加载失败不影响其他，仅记录警告
          console.warn(`Failed to load provider ${p.provider_id}:`, error)
          return { provider: p, detail: null as ProviderWithModels | null }
        }
      })
    )

    const out: Group[] = []
    for (const item of details) {
      const embeddingGroup = item.detail?.model_groups?.find((g) => g.type === 'embedding')
      // 只取 enabled 的向量模型
      const models = (embeddingGroup?.models || []).filter((m) => m.enabled)
      if (models.length > 0) {
        out.push({ provider: item.provider, models })
      }
    }
    embeddingGroups.value = out
  } catch (error) {
    console.error('Failed to load embedding model list:', error)
    toast.error(getErrorMessage(error) || t('knowledge.providersLoadFailed'))
    embeddingGroups.value = []
  } finally {
    loading.value = false
  }
}

const loadCurrentSettings = async () => {
  try {
    const [p, m, d] = await Promise.all([
      SettingsService.Get('embedding_provider_id'),
      SettingsService.Get('embedding_model_id'),
      SettingsService.Get('embedding_dimension'),
    ])
    const providerId = p?.value || ''
    const modelId = m?.value || ''
    const dim = d?.value || '1536'
    embeddingDimension.value = dim
    if (providerId && modelId) {
      embeddingSelectedKey.value = clearUnavailableChatwikiSelection(
        `${providerId}::${modelId}`,
        chatwikiAvailability.value
      )
      if (!embeddingSelectedKey.value) {
        await Promise.all([
          SettingsService.SetValue('embedding_provider_id', ''),
          SettingsService.SetValue('embedding_model_id', ''),
        ])
      }
    }
  } catch (error) {
    console.error('Failed to load embedding settings:', error)
  }
}

const ensureDefaultSelection = () => {
  // 已有选择且仍可用 -> 保持
  if (embeddingSelectedKey.value && isEmbeddingSelectionAvailable.value) return
  embeddingSelectedKey.value = getFirstSelectableModelKey(
    embeddingGroups.value.map((group) => ({
      provider: group.provider,
      model_groups: [{ type: 'embedding', models: group.models }],
    })),
    'embedding',
    chatwikiAvailability.value
  )
}

watch(embeddingSelectedKey, (selectedKey, previousSelectedKey) => {
  if (!selectedKey || selectedKey === previousSelectedKey) return

  const previousDefaultDimension = previousSelectedKey
    ? getDefaultEmbeddingDimension(previousSelectedKey)
    : DEFAULT_EMBEDDING_DIMENSION
  const shouldResetDimension =
    !embeddingDimension.value || embeddingDimension.value === previousDefaultDimension

  if (shouldResetDimension) {
    embeddingDimension.value = getDefaultEmbeddingDimension(selectedKey)
  }
})

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    embeddingSelectedKey.value = ''
    embeddingDimension.value = DEFAULT_EMBEDDING_DIMENSION
    await Promise.all([loadGroups(), loadCurrentSettings()])
    ensureDefaultSelection()
  }
)

onMounted(() => {
  unsubscribeChatwikiBindingChanged = onChatwikiBindingChanged(() => {
    if (props.open) {
      void Promise.all([loadGroups(), loadCurrentSettings()]).then(() => {
        ensureDefaultSelection()
      })
    }
  })
})

onUnmounted(() => {
  unsubscribeChatwikiBindingChanged?.()
  unsubscribeChatwikiBindingChanged = null
})

const isValid = computed(() => {
  if (!isEmbeddingSelectionAvailable.value) return false
  const dim = Number.parseInt(embeddingDimension.value, 10)
  return Number.isFinite(dim) && dim > 0
})

const handleSave = async () => {
  if (!isValid.value || saving.value) return
  saving.value = true
  try {
    const [providerId, modelId] = embeddingSelectedKey.value.split('::')
    if (!providerId || !modelId) throw new Error(t('knowledge.embeddingSettings.required'))
    const dim = String(Number.parseInt(embeddingDimension.value, 10))

    await SettingsService.UpdateEmbeddingConfig({
      provider_id: providerId,
      model_id: modelId,
      dimension: Number.parseInt(dim, 10),
    })
    toast.success(t('knowledge.embeddingSettings.saved'))
    close()
  } catch (error) {
    console.error('Failed to save embedding settings:', error)
    toast.error(getErrorMessage(error) || t('knowledge.embeddingSettings.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.embeddingSettings.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <OrangeWarning :text="t('knowledge.embeddingSettings.warning')" />

        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.embeddingSettings.embeddingModel')"
            :help="t('knowledge.help.embeddingModel')"
            required
          />
          <Select
            v-model="embeddingSelectedKey"
            v-model:open="embeddingSelectOpen"
            :disabled="loading || saving"
          >
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')">
                <template v-if="embeddingCurrentLabel">
                  <span class="flex items-center gap-1.5 truncate">
                    <span class="truncate">{{ embeddingCurrentLabel }}</span>
                    <span
                      v-if="selectedProviderIsFree"
                      class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                    >
                      {{ t('assistant.chat.freeBadge') }}
                    </span>
                  </span>
                </template>
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup v-for="g in embeddingGroups" :key="g.provider.provider_id">
                <SelectLabel
                  :class="
                    [
                      'flex items-center gap-1.5',
                      g.provider.provider_id === 'chatwiki' &&
                      chatwikiAvailability === 'unbound'
                        ? 'justify-between gap-2 pr-1'
                        : '',
                    ].filter(Boolean)
                  "
                >
                  <span class="flex min-w-0 flex-1 items-center gap-1.5">
                    <span class="truncate">{{
                      formatProviderDisplayLabel(
                        g.provider.provider_id,
                        g.provider.name,
                        chatwikiAvailability
                      )
                    }}</span>
                    <span
                      v-if="isProviderFree(g)"
                      class="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                    >
                      {{ t('assistant.chat.freeBadge') }}
                    </span>
                  </span>
                  <button
                    v-if="g.provider.provider_id === 'chatwiki' && chatwikiAvailability === 'unbound'"
                    type="button"
                    class="shrink-0 border-0 bg-transparent p-0 text-xs font-medium text-[color:var(--color-blue-600)] underline-offset-2 hover:underline hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                    @click.stop="goToChatwikiLogin"
                    @pointerdown.stop
                  >
                    {{ t('assistant.chat.goToChatwikiLogin') }}
                  </button>
                </SelectLabel>
                <SelectItem
                  v-for="m in g.models"
                  :key="`${g.provider.provider_id}::${m.model_id}`"
                  :value="`${g.provider.provider_id}::${m.model_id}`"
                  :disabled="isModelSelectionDisabled(g.provider.provider_id, chatwikiAvailability)"
                >
                  {{ getEmbeddingModelLabel(g.provider.provider_id, m) }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.embeddingSettings.embeddingDimension')"
            :help="t('knowledge.help.embeddingDimension')"
            required
          />
          <Input
            v-model="embeddingDimension"
            type="number"
            min="1"
            step="1"
            :disabled="loading || saving"
          />
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <Button variant="outline" :disabled="saving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button class="gap-2" :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="size-4 shrink-0 animate-spin" />
          {{ t('knowledge.embeddingSettings.save') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
