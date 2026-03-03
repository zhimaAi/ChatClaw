<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { LibraryService, DeleteFolderInput } from '@bindings/chatclaw/internal/services/library'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  open: boolean
  folder: Folder | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  deleted: []
}>()

const { t } = useI18n()
const deleting = ref(false)

const close = () => emit('update:open', false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    deleting.value = false
  }
)

const handleDelete = async () => {
  if (!props.folder || deleting.value) return
  deleting.value = true
  try {
    await LibraryService.DeleteFolder(
      new DeleteFolderInput({
        id: props.folder.id,
      })
    )
    emit('deleted')
    toast.success(t('knowledge.folder.deleteSuccess'))
    close()
  } catch (error) {
    console.error('Failed to delete folder:', error)
    toast.error(getErrorMessage(error) || t('knowledge.folder.deleteFailed'))
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <AlertDialog :open="open" @update:open="close">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('knowledge.folder.deleteTitle') }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ t('knowledge.folder.deleteDesc', { name: folder?.name }) }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel :disabled="deleting">
          {{ t('knowledge.folder.deleteCancel') }}
        </AlertDialogCancel>
        <AlertDialogAction
          class="bg-foreground text-background hover:bg-foreground/90"
          :disabled="deleting"
          @click.prevent="handleDelete"
        >
          <LoaderCircle v-if="deleting" class="mr-2 size-4 animate-spin" />
          {{ t('knowledge.folder.deleteConfirm') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
