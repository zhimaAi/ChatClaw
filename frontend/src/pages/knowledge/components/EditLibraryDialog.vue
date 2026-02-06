<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import FieldLabel from './FieldLabel.vue'
import OrangeWarning from './OrangeWarning.vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

import type {
  Provider,
  ProviderWithModels,
  Model,
} from '@bindings/willchat/internal/services/providers'
import { ProvidersService } from '@bindings/willchat/internal/services/providers'

import type { Library } from '@bindings/willchat/internal/services/library'
import { LibraryService, UpdateLibraryInput } from '@bindings/willchat/internal/services/library'

const props = defineProps<{
  open: boolean
  library: Library | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: [library: Library]
}>()

const { t } = useI18n()

const saving = ref(false)
const loadingProviders = ref(false)

// 语义分段开关
const semanticSegmentationEnabled = ref(false)

// RAPTOR LLM 模型选择
type Group = { provider: Provider; models: Model[] }
const raptorLLMGroups = ref<Group[]>([])
const RAPTOR_LLM_NONE = '__none__'
const raptorLLMKey = ref<string>(RAPTOR_LLM_NONE)

// Default chunk settings optimized for better retrieval performance
// Smaller chunks (512 vs 1024) provide more granular semantic matching
const chunkSize = ref<string>('512')
const chunkOverlap = ref<string>('50')

const close = () => emit('update:open', false)

const currentRaptorLLMLabel = computed(() => {
  if (!raptorLLMKey.value || raptorLLMKey.value === RAPTOR_LLM_NONE) {
    return t('knowledge.create.noRaptorLLM')
  }
  const [pid, mid] = raptorLLMKey.value.split('::')
  if (!pid || !mid) return t('knowledge.create.noRaptorLLM')
  const group = raptorLLMGroups.value.find((g) => g.provider.provider_id === pid)
  const model = group?.models.find((m) => m.model_id === mid)
  return model?.name || t('knowledge.create.noRaptorLLM')
})

const loadProviders = async () => {
  loadingProviders.value = true
  try {
    const providers = (await ProvidersService.ListProviders()) || []
    const enabledProviders = providers.filter((p) => p.enabled)
    const details = await Promise.all(
      enabledProviders.map(async (p) => {
        try {
          const detail = await ProvidersService.GetProviderWithModels(p.provider_id)
          return { provider: p, detail }
        } catch (error: unknown) {
          console.warn(`Failed to load provider ${p.provider_id}:`, error)
          return { provider: p, detail: null as ProviderWithModels | null }
        }
      })
    )

    const llmOut: Group[] = []
    for (const item of details) {
      const llmGroup = item.detail?.model_groups?.find((g) => g.type === 'llm')
      const llmModels = (llmGroup?.models || []).filter((m) => m.enabled)
      if (llmModels.length > 0) {
        llmOut.push({ provider: item.provider, models: llmModels })
      }
    }
    raptorLLMGroups.value = llmOut
  } catch (error) {
    console.error('Failed to load providers:', error)
  } finally {
    loadingProviders.value = false
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    saving.value = false
    await loadProviders()

    // init from library
    chunkSize.value = String(props.library?.chunk_size ?? 1024)
    chunkOverlap.value = String(props.library?.chunk_overlap ?? 100)

    // 初始化语义分段开关
    semanticSegmentationEnabled.value = props.library?.semantic_segmentation_enabled ?? false

    // 初始化 RAPTOR LLM 模型
    if (props.library?.raptor_llm_provider_id && props.library?.raptor_llm_model_id) {
      raptorLLMKey.value = `${props.library.raptor_llm_provider_id}::${props.library.raptor_llm_model_id}`
    } else {
      raptorLLMKey.value = RAPTOR_LLM_NONE
    }
  }
)

const isValid = computed(() => {
  if (!props.library) return false
  const cs = Number.parseInt(chunkSize.value, 10)
  const co = Number.parseInt(chunkOverlap.value, 10)
  return (
    Number.isFinite(cs) && cs >= 500 && cs <= 5000 && Number.isFinite(co) && co >= 0 && co <= 1000
  )
})

const handleSave = async () => {
  if (!props.library || !isValid.value || saving.value) return
  saving.value = true
  try {
    const isRaptorNone = !raptorLLMKey.value || raptorLLMKey.value === RAPTOR_LLM_NONE
    const [raptorPid, raptorMid] = isRaptorNone ? ['', ''] : raptorLLMKey.value.split('::')

    const updated = await LibraryService.UpdateLibrary(
      props.library.id,
      new UpdateLibraryInput({
        semantic_segmentation_enabled: semanticSegmentationEnabled.value,
        raptor_llm_provider_id: raptorPid || '',
        raptor_llm_model_id: raptorMid || '',
        chunk_size: Number.parseInt(chunkSize.value, 10),
        chunk_overlap: Number.parseInt(chunkOverlap.value, 10),
      })
    )
    if (!updated) throw new Error(t('knowledge.settings.saveFailed'))
    emit('updated', updated)
    toast.success(t('knowledge.settings.saved'))
    close()
  } catch (error) {
    console.error('Failed to update library:', error)
    toast.error(getErrorMessage(error) || t('knowledge.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.settings.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <!-- 语义分段开关 -->
        <div class="flex items-center justify-between">
          <FieldLabel
            :label="t('knowledge.create.semanticSegmentation')"
            :help="t('knowledge.help.semanticSegmentation')"
          />
          <Switch v-model="semanticSegmentationEnabled" :disabled="saving" />
        </div>

        <!-- RAPTOR LLM 模型 -->
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.raptorLLMModel')"
            :help="t('knowledge.help.raptorLLMModel')"
          />
          <Select v-model="raptorLLMKey" :disabled="loadingProviders || saving">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')">
                {{ currentRaptorLLMLabel }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem :value="RAPTOR_LLM_NONE">
                {{ t('knowledge.create.noRaptorLLM') }}
              </SelectItem>
              <SelectGroup v-for="g in raptorLLMGroups" :key="g.provider.provider_id">
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
            :label="t('knowledge.create.chunkSize')"
            :help="t('knowledge.help.chunkSize')"
          />
          <Input
            v-model="chunkSize"
            type="number"
            min="500"
            max="5000"
            step="1"
            :disabled="saving"
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.chunkOverlap')"
            :help="t('knowledge.help.chunkOverlap')"
          />
          <Input
            v-model="chunkOverlap"
            type="number"
            min="0"
            max="1000"
            step="1"
            :disabled="saving"
          />
        </div>

        <OrangeWarning :text="t('knowledge.create.advancedWarning')" />
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="saving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="mr-2 size-4 animate-spin" />
          {{ t('knowledge.settings.save') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
