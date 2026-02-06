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
const name = ref('')
const saving = ref(false)
const NAME_MAX_LEN = 30

const close = () => emit('update:open', false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    name.value = props.library?.name || ''
    saving.value = false
  }
)

const isValid = computed(() => {
  if (!props.library) return false
  const n = name.value.trim()
  return n.length > 0 && n.length <= NAME_MAX_LEN
})

const handleSave = async () => {
  if (!props.library || !isValid.value || saving.value) return
  saving.value = true
  try {
    const updated = await LibraryService.UpdateLibrary(
      props.library.id,
      new UpdateLibraryInput({
        name: name.value.trim(),
      })
    )
    if (!updated) throw new Error(t('knowledge.rename.failed'))
    emit('updated', updated)
    toast.success(t('knowledge.rename.success'))
    close()
  } catch (error) {
    console.error('Failed to rename library:', error)
    toast.error(getErrorMessage(error) || t('knowledge.rename.failed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.rename.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.name')"
            :help="t('knowledge.help.name')"
            required
          />
          <Input
            v-model="name"
            :placeholder="t('knowledge.rename.placeholder')"
            :maxlength="NAME_MAX_LEN"
            :disabled="saving"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="saving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="mr-2 size-4 animate-spin" />
          {{ t('knowledge.rename.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
