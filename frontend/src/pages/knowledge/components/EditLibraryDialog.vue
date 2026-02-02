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

type Group = { provider: Provider; models: Model[] }
const rerankGroups = ref<Group[]>([])

const saving = ref(false)
const loadingProviders = ref(false)

const topK = ref<number[]>([20])
const chunkSize = ref<string>('1024')
const chunkOverlap = ref<string>('100')
const matchThreshold = ref<string>('0.5')
const rerankKey = ref<string>('') // `${providerId}::${modelId}`

const currentRerankLabel = computed(() => {
  const [pid, mid] = rerankKey.value.split('::')
  if (!pid || !mid) return ''
  const group = rerankGroups.value.find((g) => g.provider.provider_id === pid)
  const model = group?.models.find((m) => m.model_id === mid)
  return model?.name || ''
})

const close = () => emit('update:open', false)

const ensureDefaultRerank = () => {
  if (rerankKey.value) return
  const firstGroup = rerankGroups.value[0]
  const firstModel = firstGroup?.models?.[0]
  if (firstGroup && firstModel) {
    rerankKey.value = `${firstGroup.provider.provider_id}::${firstModel.model_id}`
  }
}

const loadRerankGroups = async () => {
  loadingProviders.value = true
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
      const group = item.detail?.model_groups?.find((g) => g.type === 'rerank')
      const models = group?.models || []
      if (models.length > 0) out.push({ provider: item.provider, models })
    }
    rerankGroups.value = out
  } finally {
    loadingProviders.value = false
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    saving.value = false
    await loadRerankGroups()

    // init from library
    topK.value = [props.library?.top_k ?? 20]
    chunkSize.value = String(props.library?.chunk_size ?? 1024)
    chunkOverlap.value = String(props.library?.chunk_overlap ?? 100)
    matchThreshold.value = String(props.library?.match_threshold ?? 0.5)
    if (props.library?.rerank_provider_id && props.library?.rerank_model_id) {
      rerankKey.value = `${props.library.rerank_provider_id}::${props.library.rerank_model_id}`
    } else {
      rerankKey.value = ''
    }
    ensureDefaultRerank()
  }
)

const isValid = computed(() => {
  if (!props.library) return false
  const [pid, mid] = rerankKey.value.split('::')
  if (!pid || !mid) return false
  const cs = Number.parseInt(chunkSize.value, 10)
  const co = Number.parseInt(chunkOverlap.value, 10)
  const mt = Number.parseFloat(matchThreshold.value)
  return (
    (topK.value[0] ?? 0) > 0 &&
    Number.isFinite(cs) &&
    cs > 0 &&
    Number.isFinite(co) &&
    co >= 0 &&
    Number.isFinite(mt) &&
    mt >= 0 &&
    mt <= 1
  )
})

const handleSave = async () => {
  if (!props.library || !isValid.value || saving.value) return
  saving.value = true
  try {
    const [pid, mid] = rerankKey.value.split('::')
    const updated = await LibraryService.UpdateLibrary(
      props.library.id,
      new UpdateLibraryInput({
        rerank_provider_id: pid,
        rerank_model_id: mid,
        top_k: topK.value[0] ?? 20,
        chunk_size: Number.parseInt(chunkSize.value, 10),
        chunk_overlap: Number.parseInt(chunkOverlap.value, 10),
        match_threshold: Number.parseFloat(matchThreshold.value),
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
    <DialogContent class="sm:max-w-[560px]">
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
            :disabled="loadingProviders || saving"
            :marks="[
              { value: 1, label: '1' },
              { value: 20, label: t('knowledge.create.defaultMark'), emphasize: true },
              { value: 30, label: '30' },
              { value: 50, label: '50' },
            ]"
          />
        </div>

        <!-- rerank -->
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.rerankModel')"
            :help="t('knowledge.help.rerankModel')"
          />
          <Select
            v-model="rerankKey"
            :disabled="loadingProviders || saving || rerankGroups.length === 0"
          >
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')">
                {{ currentRerankLabel }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup v-for="g in rerankGroups" :key="g.provider.provider_id">
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
          <Input v-model="chunkSize" type="number" min="1" step="1" :disabled="saving" />
        </div>
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.chunkOverlap')"
            :help="t('knowledge.help.chunkOverlap')"
          />
          <Input v-model="chunkOverlap" type="number" min="0" step="1" :disabled="saving" />
        </div>
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.matchThreshold')"
            :help="t('knowledge.help.matchThreshold')"
          />
          <Input
            v-model="matchThreshold"
            type="number"
            min="0"
            max="1"
            step="0.01"
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
