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
} from '@bindings/willchat/internal/services/providers'
import { ProvidersService } from '@bindings/willchat/internal/services/providers'
import { SettingsService } from '@bindings/willchat/internal/services/settings'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)

type Group = { provider: Provider; models: Model[] }
const groups = ref<Group[]>([])

const selectedKey = ref<string>('') // `${providerId}::${modelId}`
const embeddingDimension = ref<string>('1536')

const currentLabel = computed(() => {
  const [pid, mid] = selectedKey.value.split('::')
  if (!pid || !mid) return ''
  const provider = groups.value.find((g) => g.provider.provider_id === pid)
  const model = provider?.models.find((m) => m.model_id === mid)
  return model?.name || ''
})

const close = () => emit('update:open', false)

const loadGroups = async () => {
  loading.value = true
  try {
    const providers = (await ProvidersService.ListProviders()) || []
    const details = await Promise.all(
      providers.map(async (p) => {
        try {
          const detail = await ProvidersService.GetProviderWithModels(p.provider_id)
          return { provider: p, detail }
        } catch {
          return { provider: p, detail: null as ProviderWithModels | null }
        }
      })
    )

    const out: Group[] = []
    for (const item of details) {
      const embeddingGroup = item.detail?.model_groups?.find((g) => g.type === 'embedding')
      const models = embeddingGroup?.models || []
      if (models.length > 0) {
        out.push({ provider: item.provider, models })
      }
    }
    groups.value = out
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
      selectedKey.value = `${providerId}::${modelId}`
    }
  } catch (error) {
    console.error('Failed to load embedding settings:', error)
  }
}

const ensureDefaultSelection = () => {
  if (selectedKey.value) return
  const first = groups.value[0]?.models[0]
  const pid = groups.value[0]?.provider.provider_id
  if (first && pid) {
    selectedKey.value = `${pid}::${first.model_id}`
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    selectedKey.value = ''
    embeddingDimension.value = '1536'
    await Promise.all([loadGroups(), loadCurrentSettings()])
    ensureDefaultSelection()
  }
)

const isValid = computed(() => {
  const [pid, mid] = selectedKey.value.split('::')
  if (!pid || !mid) return false
  const dim = Number.parseInt(embeddingDimension.value, 10)
  return Number.isFinite(dim) && dim > 0
})

const handleSave = async () => {
  if (!isValid.value || saving.value) return
  saving.value = true
  try {
    const [providerId, modelId] = selectedKey.value.split('::')
    const dim = String(Number.parseInt(embeddingDimension.value, 10))
    await Promise.all([
      SettingsService.SetValue('embedding_provider_id', providerId),
      SettingsService.SetValue('embedding_model_id', modelId),
      SettingsService.SetValue('embedding_dimension', dim),
    ])
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
    <DialogContent class="sm:max-w-[560px]">
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
          <Select v-model="selectedKey" :disabled="loading || saving">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')">
                {{ currentLabel }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup v-for="g in groups" :key="g.provider.provider_id">
                <SelectLabel>{{ g.provider.name }}</SelectLabel>
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
