<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import SliderWithMarks from './SliderWithMarks.vue'
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

// 语义分段模型选择
type Group = { provider: Provider; models: Model[] }
const semanticSegmentGroups = ref<Group[]>([])
const SEMANTIC_SEGMENT_NONE = '__none__'
const semanticSegmentKey = ref<string>(SEMANTIC_SEGMENT_NONE)

const topK = ref<number[]>([20])
const chunkSize = ref<string>('1024')
const chunkOverlap = ref<string>('100')

const close = () => emit('update:open', false)

const currentSemanticSegmentLabel = computed(() => {
  if (!semanticSegmentKey.value || semanticSegmentKey.value === SEMANTIC_SEGMENT_NONE) {
    return t('knowledge.create.noSemanticSegment')
  }
  const [pid, mid] = semanticSegmentKey.value.split('::')
  if (!pid || !mid) return t('knowledge.create.noSemanticSegment')
  const group = semanticSegmentGroups.value.find((g) => g.provider.provider_id === pid)
  const model = group?.models.find((m) => m.model_id === mid)
  return model?.name || t('knowledge.create.noSemanticSegment')
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

    const segOut: Group[] = []
    for (const item of details) {
      const llmGroup = item.detail?.model_groups?.find((g) => g.type === 'llm')
      const llmModels = (llmGroup?.models || []).filter((m) => m.enabled)
      if (llmModels.length > 0) {
        segOut.push({ provider: item.provider, models: llmModels })
      }
    }
    semanticSegmentGroups.value = segOut
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
    topK.value = [props.library?.top_k ?? 20]
    chunkSize.value = String(props.library?.chunk_size ?? 1024)
    chunkOverlap.value = String(props.library?.chunk_overlap ?? 100)

    // 初始化语义分段模型
    if (props.library?.semantic_segment_provider_id && props.library?.semantic_segment_model_id) {
      semanticSegmentKey.value = `${props.library.semantic_segment_provider_id}::${props.library.semantic_segment_model_id}`
    } else {
      semanticSegmentKey.value = SEMANTIC_SEGMENT_NONE
    }
  }
)

const isValid = computed(() => {
  if (!props.library) return false
  const cs = Number.parseInt(chunkSize.value, 10)
  const co = Number.parseInt(chunkOverlap.value, 10)
  return (
    (topK.value[0] ?? 0) > 0 &&
    Number.isFinite(cs) &&
    cs >= 500 &&
    cs <= 5000 &&
    Number.isFinite(co) &&
    co >= 0 &&
    co <= 1000
  )
})

const handleSave = async () => {
  if (!props.library || !isValid.value || saving.value) return
  saving.value = true
  try {
    const isNone = !semanticSegmentKey.value || semanticSegmentKey.value === SEMANTIC_SEGMENT_NONE
    const [pid, mid] = isNone ? ['', ''] : semanticSegmentKey.value.split('::')

    const updated = await LibraryService.UpdateLibrary(
      props.library.id,
      new UpdateLibraryInput({
        semantic_segment_provider_id: pid || '',
        semantic_segment_model_id: mid || '',
        top_k: topK.value[0] ?? 20,
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
        <!-- topK -->
        <div class="flex flex-col gap-1.5">
          <div class="flex items-center justify-between">
            <FieldLabel :label="t('knowledge.create.topK')" :help="t('knowledge.help.topK')" />
            <div class="text-sm text-muted-foreground tabular-nums">{{ topK[0] ?? 20 }}</div>
          </div>
          <SliderWithMarks
            v-model="topK"
            :min="1"
            :max="50"
            :step="1"
            :disabled="saving"
            :marks="[
              { value: 1, label: '1' },
              { value: 20, label: t('knowledge.create.defaultMark'), emphasize: true },
              { value: 30, label: '30' },
              { value: 50, label: '50' },
            ]"
          />
        </div>

        <!-- 语义分段模型 -->
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.semanticSegmentModel')"
            :help="t('knowledge.help.semanticSegmentModel')"
          />
          <Select
            v-model="semanticSegmentKey"
            :disabled="loadingProviders || saving"
          >
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')">
                {{ currentSemanticSegmentLabel }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem :value="SEMANTIC_SEGMENT_NONE">
                {{ t('knowledge.create.noSemanticSegment') }}
              </SelectItem>
              <SelectGroup v-for="g in semanticSegmentGroups" :key="g.provider.provider_id">
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
