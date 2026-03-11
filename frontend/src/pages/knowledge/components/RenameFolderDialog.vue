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
import { LibraryService, RenameFolderInput } from '@bindings/chatclaw/internal/services/library'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  open: boolean
  folder: Folder | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: [folder: Folder]
}>()

const { t } = useI18n()
const name = ref('')
const saving = ref(false)
const NAME_MAX_LEN = 50

const close = () => emit('update:open', false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    name.value = props.folder?.name || ''
    saving.value = false
  }
)

const isValid = computed(() => {
  if (!props.folder) return false
  const n = name.value.trim()
  return n.length > 0 && n.length <= NAME_MAX_LEN
})

const handleSave = async () => {
  if (!props.folder || !isValid.value || saving.value) return
  saving.value = true
  try {
    const updated = await LibraryService.RenameFolder(
      new RenameFolderInput({
        id: props.folder.id,
        name: name.value.trim(),
      })
    )
    if (!updated) throw new Error(t('knowledge.folder.renameFailed'))
    emit('updated', updated)
    toast.success(t('knowledge.folder.renameSuccess'))
    close()
  } catch (error) {
    console.error('Failed to rename folder:', error)
    toast.error(getErrorMessage(error) || t('knowledge.folder.renameFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.folder.renameTitle') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.name')"
            :help="t('knowledge.folder.nameHelp')"
            required
          />
          <Input
            v-model="name"
            :placeholder="t('knowledge.folder.renamePlaceholder')"
            :maxlength="NAME_MAX_LEN"
            :disabled="saving"
            @keyup.enter="handleSave"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="saving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button class="gap-2" :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="size-4 shrink-0 animate-spin" />
          {{ t('knowledge.rename.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
