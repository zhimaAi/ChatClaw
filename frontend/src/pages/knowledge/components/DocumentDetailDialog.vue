<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FileText, RefreshCw, Trash2, FolderPlus, Pencil } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { DocumentService } from '@bindings/chatclaw/internal/services/document'
import type { Document as BackendDocument } from '@bindings/chatclaw/internal/services/document'
import type { Folder } from '@bindings/chatclaw/internal/services/library'
import type { Document } from './DocumentCard.vue'

const props = defineProps<{
  open: boolean
  document: Document | null
  folders: Folder[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  relearn: [doc: Document]
  'move-to-folder': [doc: Document]
  rename: [doc: Document]
  delete: [doc: Document]
}>()

const { t } = useI18n()
const detailDoc = ref<BackendDocument | null>(null)
const loading = ref(false)

const close = () => emit('update:open', false)

watch(
  () => props.open,
  async (open) => {
    if (!open) {
      detailDoc.value = null
      return
    }
    if (!props.document) return

    loading.value = true
    try {
      detailDoc.value = await DocumentService.GetDocument(props.document.id)
    } catch (error) {
      console.error('Failed to load document detail:', error)
      toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
    } finally {
      loading.value = false
    }
  }
)

const formatFileSize = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`
}

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const folderName = computed(() => {
  if (!detailDoc.value?.folder_id) return t('knowledge.detail.uncategorized')
  const folder = props.folders.find((f) => f.id === detailDoc.value!.folder_id!)
  return folder?.name || t('knowledge.detail.uncategorized')
})

const statusText = computed(() => {
  if (!detailDoc.value) return ''
  const parsingStatus = detailDoc.value.parsing_status
  const embeddingStatus = detailDoc.value.embedding_status

  if (parsingStatus === 3 || embeddingStatus === 3) {
    return t('knowledge.content.status.failed')
  }
  if (embeddingStatus === 2) {
    return t('knowledge.content.status.completed')
  }
  if (embeddingStatus === 1) {
    return `${detailDoc.value.embedding_progress}% ${t('knowledge.content.status.learning')}`
  }
  if (parsingStatus === 1) {
    return `${detailDoc.value.parsing_progress}% ${t('knowledge.content.status.parsing')}`
  }
  return t('knowledge.content.status.pending')
})

const handleMoveToFolder = () => {
  if (!props.document) return
  close() // Close detail dialog first
  emit('move-to-folder', props.document)
}

const handleRename = () => {
  if (!props.document) return
  close() // Close detail dialog first
  emit('rename', props.document)
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="lg" class="max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.detail.title') }}</DialogTitle>
      </DialogHeader>

      <div v-if="loading" class="flex items-center justify-center py-8">
        <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
      </div>

      <div v-else-if="detailDoc" class="flex flex-col gap-6 py-4">
        <!-- 基础信息 -->
        <div class="flex flex-col gap-3">
          <h3 class="text-sm font-medium text-foreground">{{ t('knowledge.detail.basicInfo') }}</h3>
          <div class="grid grid-cols-2 gap-4 text-sm">
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.filename') }}</span>
              <span class="text-foreground">{{ detailDoc.original_name }}</span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.size') }}</span>
              <span class="text-foreground">{{ formatFileSize(detailDoc.file_size) }}</span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.type') }}</span>
              <span class="text-foreground">{{ detailDoc.extension.toUpperCase() }}</span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.createdAt') }}</span>
              <span class="text-foreground">{{ formatDate(detailDoc.created_at) }}</span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.source') }}</span>
              <span class="text-foreground">
                {{
                  detailDoc.source_type === 'local'
                    ? t('knowledge.detail.local')
                    : t('knowledge.detail.web')
                }}
              </span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.folder') }}</span>
              <span class="text-foreground">{{ folderName }}</span>
            </div>
          </div>
        </div>

        <!-- 处理状态 -->
        <div class="flex flex-col gap-3">
          <h3 class="text-sm font-medium text-foreground">
            {{ t('knowledge.detail.processingInfo') }}
          </h3>
          <div class="flex flex-col gap-2 text-sm">
            <div class="flex items-center justify-between">
              <span class="text-muted-foreground">{{ t('knowledge.detail.status.parsing') }}</span>
              <span class="text-foreground">{{ statusText }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-muted-foreground">{{
                t('knowledge.detail.status.embedding')
              }}</span>
              <span class="text-foreground">{{ statusText }}</span>
            </div>
            <div
              v-if="detailDoc.parsing_error || detailDoc.embedding_error"
              class="mt-2 rounded-md border border-border bg-muted/50 p-2 text-xs text-muted-foreground"
            >
              {{ detailDoc.parsing_error || detailDoc.embedding_error }}
            </div>
          </div>
        </div>

        <!-- 统计信息 -->
        <div class="flex flex-col gap-3">
          <h3 class="text-sm font-medium text-foreground">{{ t('knowledge.detail.stats') }}</h3>
          <div class="grid grid-cols-2 gap-4 text-sm">
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{ t('knowledge.detail.field.wordTotal') }}</span>
              <span class="text-foreground">{{ detailDoc.word_total.toLocaleString() }}</span>
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-muted-foreground">{{
                t('knowledge.detail.field.splitTotal')
              }}</span>
              <span class="text-foreground">{{ detailDoc.split_total.toLocaleString() }}</span>
            </div>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="flex flex-wrap gap-2 border-t border-border pt-4">
          <Button variant="outline" size="sm" class="gap-2" @click="emit('relearn', document!)">
            <RefreshCw class="size-4" />
            {{ t('knowledge.detail.actions.relearn') }}
          </Button>
          <Button variant="outline" size="sm" class="gap-2" @click="handleMoveToFolder">
            <FolderPlus class="size-4" />
            {{ t('knowledge.detail.actions.moveToFolder') }}
          </Button>
          <Button variant="outline" size="sm" class="gap-2" @click="handleRename">
            <Pencil class="size-4" />
            {{ t('knowledge.detail.actions.rename') }}
          </Button>
          <Button
            variant="outline"
            size="sm"
            class="gap-2 text-destructive"
            @click="emit('delete', document!)"
          >
            <Trash2 class="size-4" />
            {{ t('knowledge.detail.actions.delete') }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
