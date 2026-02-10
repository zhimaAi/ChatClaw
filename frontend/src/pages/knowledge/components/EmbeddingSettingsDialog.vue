<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
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
} from '@bindings/willclaw/internal/services/providers'
import { ProvidersService } from '@bindings/willclaw/internal/services/providers'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)

type Group = { provider: Provider; models: Model[] }
const embeddingGroups = ref<Group[]>([])

const embeddingSelectedKey = ref<string>('') // `${providerId}::${modelId}`
const embeddingDimension = ref<string>('1536')

const embeddingCurrentLabel = computed(() => {
  const [pid, mid] = embeddingSelectedKey.value.split('::')
  if (!pid || !mid) return ''
  const provider = embeddingGroups.value.find((g) => g.provider.provider_id === pid)
  const model = provider?.models.find((m) => m.model_id === mid)
  return model?.name || ''
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

const isEmbeddingSelectionAvailable = computed(() => {
  const [pid, mid] = embeddingSelectedKey.value.split('::')
  if (!pid || !mid) return false
  const provider = embeddingGroups.value.find((g) => g.provider.provider_id === pid)
  return !!provider?.models.some((m) => m.model_id === mid)
})

const loadGroups = async () => {
  loading.value = true
  try {
    const providers = (await ProvidersService.ListProviders()) || []
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
      embeddingSelectedKey.value = `${providerId}::${modelId}`
    }
  } catch (error) {
    console.error('Failed to load embedding settings:', error)
  }
}

const ensureDefaultSelection = () => {
  // 已有选择且仍可用 -> 保持
  if (embeddingSelectedKey.value && isEmbeddingSelectionAvailable.value) return
  const first = embeddingGroups.value[0]?.models[0]
  const pid = embeddingGroups.value[0]?.provider.provider_id
  if (first && pid) {
    embeddingSelectedKey.value = `${pid}::${first.model_id}`
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    embeddingSelectedKey.value = ''
    embeddingDimension.value = '1536'
    await Promise.all([loadGroups(), loadCurrentSettings()])
    ensureDefaultSelection()
  }
)

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
          <Select v-model="embeddingSelectedKey" :disabled="loading || saving">
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
                <SelectLabel class="flex items-center gap-1.5">
                  <span>{{ g.provider.name }}</span>
                  <span
                    v-if="isProviderFree(g)"
                    class="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                  >
                    {{ t('assistant.chat.freeBadge') }}
                  </span>
                </SelectLabel>
                <SelectItem
                  v-for="m in g.models"
                  :key="`${g.provider.provider_id}::${m.model_id}`"
                  :value="`${g.provider.provider_id}::${m.model_id}`"
                >
                  {{ m.name }}
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
        <Button :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="mr-2 size-4 animate-spin" />
          {{ t('knowledge.embeddingSettings.save') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
