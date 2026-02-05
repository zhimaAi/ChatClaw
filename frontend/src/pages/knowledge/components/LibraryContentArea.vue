<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialogs, Events } from '@wailsio/runtime'
import { Search, Upload, Plus } from 'lucide-vue-next'
import IconUploadFile from '@/assets/icons/upload-file.svg'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import DocumentCard from './DocumentCard.vue'
import RenameDocumentDialog from './RenameDocumentDialog.vue'
import type { Document, DocumentStatus } from './DocumentCard.vue'
import type { Library } from '@bindings/willchat/internal/services/library'
import {
  DocumentService,
  type Document as BackendDocument,
} from '@bindings/willchat/internal/services/document'

// 进度事件数据（从后端接收）
interface ProgressEvent {
  document_id: number
  library_id: number
  parsing_status: number
  parsing_progress: number
  parsing_error: string
  embedding_status: number
  embedding_progress: number
  embedding_error: string
}

// 缩略图事件数据（从后端接收）
interface ThumbnailEvent {
  document_id: number
  library_id: number
  thumb_icon: string
}

const props = defineProps<{
  library: Library
}>()

const { t } = useI18n()

const searchQuery = ref('')
const deleteDialogOpen = ref(false)
const documentToDelete = ref<Document | null>(null)
const renameDialogOpen = ref(false)
const documentToRename = ref<Document | null>(null)
const documents = ref<Document[]>([])
const isLoading = ref(false)

// 状态常量
const STATUS_PENDING = 0
const STATUS_PROCESSING = 1
const STATUS_COMPLETED = 2
const STATUS_FAILED = 3

// 将后端文档转换为前端文档格式
const convertDocument = (doc: BackendDocument): Document => {
  let status: DocumentStatus = 'pending'
  let progress = 0
  let errorMessage = ''

  // 根据解析和嵌入状态确定整体状态
  if (doc.parsing_status === STATUS_FAILED || doc.embedding_status === STATUS_FAILED) {
    status = 'failed'
    // 提取错误信息
    errorMessage = doc.parsing_error || doc.embedding_error || ''
  } else if (doc.embedding_status === STATUS_COMPLETED) {
    status = 'completed'
  } else if (doc.embedding_status === STATUS_PROCESSING) {
    status = 'learning'
    progress = doc.embedding_progress
  } else if (doc.parsing_status === STATUS_PROCESSING) {
    status = 'parsing'
    progress = doc.parsing_progress
  } else if (doc.parsing_status === STATUS_COMPLETED) {
    status = 'learning'
    progress = 0
  }

  return {
    id: doc.id,
    contentHash: doc.content_hash,
    name: doc.original_name,
    fileType: doc.extension,
    createdAt: doc.created_at,
    status,
    progress,
    errorMessage,
    thumbIcon: doc.thumb_icon || undefined,
    fileMissing: doc.file_missing || false,
  }
}

// 加载文档列表
const loadDocuments = async () => {
  if (!props.library?.id) return

  isLoading.value = true
  try {
    const result = await DocumentService.ListDocuments(props.library.id, searchQuery.value)
    documents.value = result.map(convertDocument)
  } catch (error) {
    console.error('Failed to load documents:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.loadFailed'))
  } finally {
    isLoading.value = false
  }
}

// 监听知识库变化，重新加载文档
watch(
  () => props.library?.id,
  () => {
    searchQuery.value = ''
    loadDocuments()
  },
  { immediate: true },
)

// 搜索防抖
let searchTimeout: ReturnType<typeof setTimeout> | null = null
watch(searchQuery, () => {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    loadDocuments()
  }, 300)
})

const filteredDocuments = computed(() => {
  return documents.value
})

const handleAddDocument = async () => {
  try {
    const result = await Dialogs.OpenFile({
      Title: t('knowledge.content.selectFile'),
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: true,
      Filters: [
        {
          DisplayName: t('knowledge.content.fileTypes.documents'),
          Pattern: '*.pdf;*.doc;*.docx;*.txt;*.md;*.csv;*.xlsx;*.html;*.htm;*.ofd',
        },
        {
          DisplayName: t('knowledge.content.fileTypes.all'),
          Pattern: '*.*',
        },
      ],
    })
    if (result && result.length > 0) {
      // 上传文件
      const uploaded = await DocumentService.UploadDocuments({
        library_id: props.library.id,
        file_paths: result,
      })

      // 直接将上传的文档添加到列表（避免等待全量刷新）
      for (const doc of uploaded) {
        const converted = convertDocument(doc as unknown as BackendDocument)
        // 覆盖上传时会删除旧记录再插入新记录（ID 会变），按 content_hash 去重才是稳定的
        const hash = (doc as unknown as BackendDocument).content_hash
        const existingIndex =
          hash && hash.trim()
            ? documents.value.findIndex((d) => d.contentHash === hash)
            : documents.value.findIndex((d) => d.id === (doc as unknown as BackendDocument).id)
        if (existingIndex >= 0) {
          // 更新已存在的文档（覆盖上传）
          documents.value[existingIndex] = converted
        } else {
          // 添加新文档到列表顶部
          documents.value.unshift(converted)
        }
      }

      toast.success(t('knowledge.content.upload.count', { count: uploaded.length }))
    }
  } catch (error) {
    console.error('Failed to upload documents:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.upload.failed'))
  }
}

const handleRename = (doc: Document) => {
  documentToRename.value = doc
  renameDialogOpen.value = true
}

const confirmRename = async (doc: Document | null, newName: string) => {
  if (!doc) return

  try {
    const updated = await DocumentService.RenameDocument({
      id: doc.id,
      new_name: newName,
    })

    // 更新列表中的文档
    if (updated) {
      const index = documents.value.findIndex((d) => d.id === doc.id)
      if (index !== -1) {
        documents.value[index] = convertDocument(updated)
      }
    }

    renameDialogOpen.value = false
    documentToRename.value = null

    toast.success(t('knowledge.content.rename.success'))
  } catch (error) {
    console.error('Failed to rename document:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.rename.failed'))
  }
}

const handleRelearn = async (doc: Document) => {
  try {
    // Call backend to reprocess document
    await DocumentService.ReprocessDocument(doc.id)

    // Update document status to pending/learning
    const index = documents.value.findIndex((d) => d.id === doc.id)
    if (index !== -1) {
      documents.value[index] = {
        ...documents.value[index],
        status: 'pending',
        progress: 0,
        errorMessage: '',
      }
    }

    toast.success(t('knowledge.content.relearn.success'))
  } catch (error) {
    console.error('Failed to relearn document:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.relearn.failed'))
  }
}

const handleOpenDelete = (doc: Document) => {
  documentToDelete.value = doc
  deleteDialogOpen.value = true
}

const confirmDelete = async () => {
  if (!documentToDelete.value) return

  try {
    await DocumentService.DeleteDocument(documentToDelete.value.id)
    documents.value = documents.value.filter((d) => d.id !== documentToDelete.value?.id)

    toast.success(t('knowledge.content.delete.success'))
  } catch (error) {
    console.error('Failed to delete document:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.delete.failed'))
  } finally {
    deleteDialogOpen.value = false
    documentToDelete.value = null
  }
}

// 监听文档进度事件
let unsubscribeProgress: (() => void) | null = null
let unsubscribeThumbnail: (() => void) | null = null

onMounted(() => {
  // 监听缩略图更新事件
  unsubscribeThumbnail = Events.On('document:thumbnail', (event: { data: ThumbnailEvent }) => {
    const thumbnail = event.data
    // 只更新当前知识库的文档
    if (thumbnail.library_id !== props.library?.id) return

    const index = documents.value.findIndex((d) => d.id === thumbnail.document_id)
    if (index === -1) return

    // 更新文档缩略图
    documents.value[index] = {
      ...documents.value[index],
      thumbIcon: thumbnail.thumb_icon || undefined,
    }
  })

  unsubscribeProgress = Events.On('document:progress', (event: { data: ProgressEvent }) => {
    const progress = event.data
    // 只更新当前知识库的文档
    if (progress.library_id !== props.library?.id) return

    const index = documents.value.findIndex((d) => d.id === progress.document_id)
    if (index === -1) return

    // 更新文档状态
    let status: DocumentStatus = 'pending'
    let progressValue = 0
    let errorMessage = ''

    if (progress.parsing_status === STATUS_FAILED || progress.embedding_status === STATUS_FAILED) {
      status = 'failed'
      errorMessage = progress.parsing_error || progress.embedding_error || ''
    } else if (progress.embedding_status === STATUS_COMPLETED) {
      status = 'completed'
    } else if (progress.embedding_status === STATUS_PROCESSING) {
      status = 'learning'
      progressValue = progress.embedding_progress
    } else if (progress.parsing_status === STATUS_PROCESSING) {
      status = 'parsing'
      progressValue = progress.parsing_progress
    } else if (progress.parsing_status === STATUS_COMPLETED) {
      status = 'learning'
      progressValue = 0
    }

    documents.value[index] = {
      ...documents.value[index],
      status,
      progress: progressValue,
      errorMessage,
    }
  })
})

onUnmounted(() => {
  if (unsubscribeProgress) {
    unsubscribeProgress()
  }
  if (unsubscribeThumbnail) {
    unsubscribeThumbnail()
  }
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
})
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- 头部区域 -->
    <div class="flex h-12 items-center justify-between px-4">
      <h2 class="text-base font-medium text-foreground">{{ library.name }}</h2>
      <div class="flex items-center gap-1.5">
        <!-- 搜索框 -->
        <div class="relative w-40">
          <Search class="absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground/50" />
          <Input
            v-model="searchQuery"
            type="text"
            :placeholder="t('knowledge.content.searchPlaceholder')"
            class="h-6 pr-7 text-xs placeholder:text-muted-foreground/40"
          />
        </div>
        <!-- 添加文档按钮 -->
        <Button
          variant="ghost"
          size="icon"
          class="size-6"
          :title="t('knowledge.content.addDocument')"
          @click="handleAddDocument"
        >
          <IconUploadFile class="size-4 text-muted-foreground" />
        </Button>
      </div>
    </div>

    <!-- 内容区域 -->
    <div class="flex-1 overflow-auto p-4">
      <!-- 加载中 -->
      <div v-if="isLoading" class="flex h-full items-center justify-center">
        <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
      </div>

      <!-- 空状态 -->
      <div
        v-else-if="filteredDocuments.length === 0"
        class="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground"
      >
        <Upload class="size-10 opacity-40" />
        <div class="text-center">
          <p class="text-sm">{{ t('knowledge.content.empty.title') }}</p>
          <p class="mt-1 text-xs text-muted-foreground/70">{{ t('knowledge.content.empty.desc') }}</p>
        </div>
        <Button variant="outline" size="sm" class="gap-1.5" @click="handleAddDocument">
          <Plus class="size-4" />
          {{ t('knowledge.content.addDocument') }}
        </Button>
      </div>

      <!-- 文档网格 -->
      <div
        v-else
        class="grid auto-rows-max gap-4"
        style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
      >
        <DocumentCard
          v-for="doc in filteredDocuments"
          :key="doc.id"
          :document="doc"
          @rename="handleRename"
          @relearn="handleRelearn"
          @delete="handleOpenDelete"
        />
      </div>
    </div>

    <!-- 重命名对话框 -->
    <RenameDocumentDialog
      v-model:open="renameDialogOpen"
      :document="documentToRename"
      @confirm="confirmRename"
    />

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('knowledge.content.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('knowledge.content.delete.desc', { name: documentToDelete?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {{ t('knowledge.content.delete.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click.prevent="confirmDelete"
          >
            {{ t('knowledge.content.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
