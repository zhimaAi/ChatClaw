<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronUp, LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
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

import type { Library } from '@bindings/willclaw/internal/services/library'
import { LibraryService, CreateLibraryInput } from '@bindings/willclaw/internal/services/library'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [library: Library]
}>()

const { t } = useI18n()

const advanced = ref(false)
const isSubmitting = ref(false)
const loadingEmbedding = ref(false)
const loadingProviders = ref(false)
const embeddingReady = ref(false)

const name = ref('')
const NAME_MAX_LEN = 30

// 语义分段开关
const semanticSegmentationEnabled = ref(false)

// RAPTOR LLM 模型选择
type Group = { provider: Provider; models: Model[] }
const raptorLLMGroups = ref<Group[]>([])
const RAPTOR_LLM_NONE = '__none__'
const raptorLLMKey = ref<string>(RAPTOR_LLM_NONE) // `${providerId}::${modelId}` or NONE

// advanced fields（用字符串承接输入，提交时再转 number）
const chunkSize = ref<string>('1024')
const chunkOverlap = ref<string>('100')

const close = () => emit('update:open', false)

const resetForm = () => {
  advanced.value = false
  isSubmitting.value = false
  name.value = ''
  semanticSegmentationEnabled.value = false
  raptorLLMKey.value = RAPTOR_LLM_NONE
  chunkSize.value = '1024'
  chunkOverlap.value = '100'
}

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

const isFormValid = computed(() => {
  const n = name.value.trim()
  if (n === '' || n.length > NAME_MAX_LEN) return false

  // 高级设置展开时校验范围
  if (advanced.value) {
    const cs = Number.parseInt(chunkSize.value, 10)
    const co = Number.parseInt(chunkOverlap.value, 10)
    if (!Number.isFinite(cs) || cs < 500 || cs > 5000) return false
    if (!Number.isFinite(co) || co < 0 || co > 1000) return false
  }

  return true
})

const canSubmit = computed(() => {
  return isFormValid.value && embeddingReady.value && !isSubmitting.value
})

const loadEmbeddingReady = async () => {
  loadingEmbedding.value = true
  try {
    const [p, m] = await Promise.all([
      SettingsService.Get('embedding_provider_id'),
      SettingsService.Get('embedding_model_id'),
    ])
    embeddingReady.value = !!(p?.value?.trim() && m?.value?.trim())
  } catch (error) {
    console.error('Failed to read embedding settings:', error)
    embeddingReady.value = false
  } finally {
    loadingEmbedding.value = false
  }
}

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
      // RAPTOR LLM 模型：仅 enabled 的 llm 模型
      const llmGroup = item.detail?.model_groups?.find((g) => g.type === 'llm')
      const llmModels = (llmGroup?.models || []).filter((m) => m.enabled)
      if (llmModels.length > 0) {
        llmOut.push({ provider: item.provider, models: llmModels })
      }
    }
    raptorLLMGroups.value = llmOut
  } catch (error) {
    console.error('Failed to load providers:', error)
    toast.error(getErrorMessage(error) || t('knowledge.providersLoadFailed'))
  } finally {
    loadingProviders.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      resetForm()
      void loadEmbeddingReady()
      void loadProviders()
    }
  }
)

const handleSubmit = async () => {
  if (!isFormValid.value || isSubmitting.value) return
  if (loadingEmbedding.value) return
  if (!embeddingReady.value) {
    toast.error(t('knowledge.embeddingSettings.required'))
    return
  }
  isSubmitting.value = true

  try {
    const toInt = (v: string) => {
      const n = Number.parseInt(v, 10)
      return Number.isFinite(n) ? n : undefined
    }
    const isRaptorNone = !raptorLLMKey.value || raptorLLMKey.value === RAPTOR_LLM_NONE
    const [raptorProviderId, raptorModelId] = isRaptorNone
      ? ['', '']
      : raptorLLMKey.value.split('::')

    const input = new CreateLibraryInput({
      name: name.value.trim(),
      semantic_segmentation_enabled: semanticSegmentationEnabled.value,
      raptor_llm_provider_id: raptorProviderId || '',
      raptor_llm_model_id: raptorModelId || '',
      chunk_size: toInt(chunkSize.value) ?? 1024,
      chunk_overlap: toInt(chunkOverlap.value) ?? 100,
    })

    const lib = await LibraryService.CreateLibrary(input)
    if (!lib) {
      throw new Error(t('knowledge.create.failed'))
    }
    emit('created', lib)
    close()
  } catch (error) {
    console.error('Failed to create library:', error)
    toast.error(getErrorMessage(error) || t('knowledge.create.failed'))
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.create.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <!-- 名称 -->
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.name')"
            :help="t('knowledge.help.name')"
            required
          />
          <Input
            v-model="name"
            :placeholder="t('knowledge.create.namePlaceholder')"
            :maxlength="NAME_MAX_LEN"
            :disabled="isSubmitting"
          />
        </div>

        <!-- 高级设置 -->
        <div
          v-if="advanced"
          class="mt-1 flex flex-col gap-4 rounded-lg border border-border bg-muted/20 p-4"
        >
          <div class="text-base font-semibold text-foreground">
            {{ t('knowledge.create.advanced') }}
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
              :disabled="isSubmitting"
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
              :disabled="isSubmitting"
            />
          </div>

          <!-- 语义分段开关 -->
          <div class="flex items-center justify-between">
            <FieldLabel
              :label="t('knowledge.create.semanticSegmentation')"
              :help="t('knowledge.help.semanticSegmentation')"
            />
            <Switch v-model="semanticSegmentationEnabled" :disabled="isSubmitting" />
          </div>

          <!-- RAPTOR LLM 模型 -->
          <div class="flex flex-col gap-1.5">
            <FieldLabel
              :label="t('knowledge.create.raptorLLMModel')"
              :help="t('knowledge.help.raptorLLMModel')"
            />
            <Select v-model="raptorLLMKey" :disabled="loadingProviders || isSubmitting">
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

          <OrangeWarning :text="t('knowledge.create.advancedWarning')" />
        </div>
      </div>

      <div class="flex items-center justify-between gap-3">
        <button
          type="button"
          class="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
          :disabled="isSubmitting"
          @click="advanced = !advanced"
        >
          <span>{{
            advanced ? t('knowledge.create.advancedHide') : t('knowledge.create.advanced')
          }}</span>
          <ChevronUp v-if="advanced" class="size-4" />
          <ChevronDown v-else class="size-4" />
        </button>

        <div class="flex items-center gap-2">
          <Button variant="outline" :disabled="isSubmitting" @click="close">
            {{ t('knowledge.create.cancel') }}
          </Button>
          <Button :disabled="!canSubmit" @click="handleSubmit">
            <LoaderCircle v-if="isSubmitting" class="mr-2 size-4 animate-spin" />
            {{ t('knowledge.create.confirm') }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
