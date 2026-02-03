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

import FieldLabel from './FieldLabel.vue'
import SliderWithMarks from './SliderWithMarks.vue'
import OrangeWarning from './OrangeWarning.vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

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

const topK = ref<number[]>([20])
const chunkSize = ref<string>('1024')
const chunkOverlap = ref<string>('100')
const matchThreshold = ref<string>('0.5')

const close = () => emit('update:open', false)

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    saving.value = false

    // init from library
    topK.value = [props.library?.top_k ?? 20]
    chunkSize.value = String(props.library?.chunk_size ?? 1024)
    chunkOverlap.value = String(props.library?.chunk_overlap ?? 100)
    matchThreshold.value = String(props.library?.match_threshold ?? 0.5)
  }
)

const isValid = computed(() => {
  if (!props.library) return false
  const cs = Number.parseInt(chunkSize.value, 10)
  const co = Number.parseInt(chunkOverlap.value, 10)
  const mt = Number.parseFloat(matchThreshold.value)
  return (
    (topK.value[0] ?? 0) > 0 &&
    Number.isFinite(cs) &&
    cs >= 500 &&
    cs <= 5000 &&
    Number.isFinite(co) &&
    co >= 0 &&
    co <= 1000 &&
    Number.isFinite(mt) &&
    mt >= 0 &&
    mt <= 1
  )
})

const handleSave = async () => {
  if (!props.library || !isValid.value || saving.value) return
  saving.value = true
  try {
    const updated = await LibraryService.UpdateLibrary(
      props.library.id,
      new UpdateLibraryInput({
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
