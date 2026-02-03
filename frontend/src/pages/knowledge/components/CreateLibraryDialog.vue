<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronUp, LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import FieldLabel from './FieldLabel.vue'
import OrangeWarning from './OrangeWarning.vue'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import SliderWithMarks from './SliderWithMarks.vue'

import type { Library } from '@bindings/willchat/internal/services/library'
import { LibraryService, CreateLibraryInput } from '@bindings/willchat/internal/services/library'
import { SettingsService } from '@bindings/willchat/internal/services/settings'

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
const embeddingReady = ref(false)

const name = ref('')
const NAME_MAX_LEN = 30

// advanced fields（用字符串承接输入，提交时再转 number）
// shadcn Slider 使用 number[] 承载（支持 range）
const topK = ref<number[]>([20])
const chunkSize = ref<string>('1024')
const chunkOverlap = ref<string>('100')
const matchThreshold = ref<string>('0.5')

const close = () => emit('update:open', false)

const resetForm = () => {
  advanced.value = false
  isSubmitting.value = false
  name.value = ''
  topK.value = [20]
  chunkSize.value = '1024'
  chunkOverlap.value = '100'
  matchThreshold.value = '0.5'
}

const isFormValid = computed(() => {
  const n = name.value.trim()
  return n !== '' && n.length <= NAME_MAX_LEN
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

watch(
  () => props.open,
  (open) => {
    if (open) {
      resetForm()
      void loadEmbeddingReady()
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
    const toFloat = (v: string) => {
      const n = Number.parseFloat(v)
      return Number.isFinite(n) ? n : undefined
    }

    const input = new CreateLibraryInput({
      name: name.value.trim(),
      top_k: topK.value[0] ?? 20,
      chunk_size: toInt(chunkSize.value) ?? 1024,
      chunk_overlap: toInt(chunkOverlap.value) ?? 100,
      match_threshold: toFloat(matchThreshold.value) ?? 0.5,
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

        <!-- 请求文档分片数量 -->
        <div class="flex flex-col gap-1.5">
          <div class="flex items-center justify-between">
            <FieldLabel :label="t('knowledge.create.topK')" :help="t('knowledge.help.topK')" />
            <div class="text-sm text-muted-foreground tabular-nums">
              {{ topK[0] ?? 20 }}
            </div>
          </div>
          <SliderWithMarks
            v-model="topK"
            :min="1"
            :max="50"
            :step="1"
            :disabled="isSubmitting"
            :marks="[
              { value: 1, label: '1' },
              { value: 20, label: t('knowledge.create.defaultMark'), emphasize: true },
              { value: 30, label: '30' },
              { value: 50, label: '50' },
            ]"
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
              :disabled="isSubmitting"
            />
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
