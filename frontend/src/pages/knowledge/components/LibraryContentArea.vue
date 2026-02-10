<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialogs, Events } from '@wailsio/runtime'
import { Search, Upload, Plus, ArrowDownNarrowWide, ArrowUpNarrowWide } from 'lucide-vue-next'
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
import type { Library } from '@bindings/willclaw/internal/services/library'
import {
  DocumentService,
  type Document as BackendDocument,
} from '@bindings/willclaw/internal/services/document'

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
const sortBy = ref<'created_desc' | 'created_asc'>('created_desc')
const deleteDialogOpen = ref(false)
const documentToDelete = ref<Document | null>(null)
const renameDialogOpen = ref(false)
const documentToRename = ref<Document | null>(null)
const documents = ref<Document[]>([])
const isLoading = ref(false)
const isLoadingMore = ref(false)
const hasMore = ref(true)
const beforeID = ref<number>(0)
const PAGE_SIZE = 100
let loadToken = 0

const isUploading = ref(false)
const uploadTotal = ref(0)
const uploadDone = ref(0)
const isDragOver = ref(false)
let unsubscribeUploadProgress: (() => void) | null = null
let unsubscribeUploaded: (() => void) | null = null
let unsubscribeFileDrop: (() => void) | null = null

const scrollContainerRef = ref<HTMLElement | null>(null)
const loadMoreSentinelRef = ref<HTMLElement | null>(null)
let loadMoreObserver: IntersectionObserver | null = null

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

const resetAndLoad = async () => {
  if (!props.library?.id) return

  loadToken += 1
  documents.value = []
  beforeID.value = 0
  hasMore.value = true // Always allow the first request to go through
  await loadMore(loadToken)
}

const loadMore = async (token?: number) => {
  if (!props.library?.id) return
  if (!hasMore.value) return
  // When searching, backend returns a single page (no cursor). Do not auto-load more.
  if (searchQuery.value.trim() && documents.value.length > 0) return
  if (isUploading.value) return
  if (isLoading.value || isLoadingMore.value) return

  const currentToken = token ?? loadToken
  const isFirst = documents.value.length === 0
  if (isFirst) {
    isLoading.value = true
  } else {
    isLoadingMore.value = true
  }

  try {
    const result = await DocumentService.ListDocumentsPage({
      library_id: props.library.id,
      keyword: searchQuery.value,
      before_id: beforeID.value,
      limit: PAGE_SIZE,
      sort_by: sortBy.value,
    })
    if (currentToken !== loadToken) return

    const incoming = result.map(convertDocument)
    const existingIDs = new Set(documents.value.map((d) => d.id))
    const merged: Document[] = []
    for (const doc of incoming) {
      if (!existingIDs.has(doc.id)) merged.push(doc)
    }

    if (merged.length > 0) {
      documents.value.push(...merged)
      beforeID.value = merged[merged.length - 1].id
    }

    if (searchQuery.value.trim()) {
      hasMore.value = false
    } else {
      hasMore.value = result.length >= PAGE_SIZE
      // If server returned duplicates only, stop to avoid spinning.
      if (result.length > 0 && merged.length === 0) {
        hasMore.value = false
      }
    }
  } catch (error) {
    console.error('Failed to load documents:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.loadFailed'))
    hasMore.value = false
  } finally {
    isLoading.value = false
    isLoadingMore.value = false
  }
}

// 监听知识库变化，重新加载文档
watch(
  () => props.library?.id,
  () => {
    searchQuery.value = ''
    resetAndLoad()
  },
  { immediate: true }
)

// 搜索防抖
let searchTimeout: ReturnType<typeof setTimeout> | null = null
watch(searchQuery, () => {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    resetAndLoad()
  }, 300)
})

const filteredDocuments = computed(() => {
  return documents.value
})

const toggleSort = () => {
  sortBy.value = sortBy.value === 'created_desc' ? 'created_asc' : 'created_desc'
  resetAndLoad()
}

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
      // 立即给用户反馈，避免“卡住”的感觉
      isUploading.value = true
      uploadTotal.value = result.length
      uploadDone.value = 0
      await nextTick()

      // 上传文件
      const uploaded = await DocumentService.UploadDocuments({
        library_id: props.library.id,
        file_paths: result,
      })

      // 上传完成后统一刷新第一页（只渲染 100 条，避免一次性渲染 500 卡片导致卡顿）
      await resetAndLoad()

      toast.success(t('knowledge.content.upload.count', { count: uploaded.length }))
    }
  } catch (error) {
    // User cancelled the file dialog — not an error
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to upload documents:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.upload.failed'))
  } finally {
    isUploading.value = false
  }
}

// Handle files dropped via Wails native file drop
const handleFileDrop = async (filePaths: string[]) => {
  if (!props.library?.id || filePaths.length === 0) return
  if (isUploading.value) return

  try {
    isUploading.value = true
    uploadTotal.value = filePaths.length
    uploadDone.value = 0
    await nextTick()

    const uploaded = await DocumentService.UploadDocuments({
      library_id: props.library.id,
      file_paths: filePaths,
    })

    await resetAndLoad()
    toast.success(t('knowledge.content.upload.count', { count: uploaded.length }))
  } catch (error) {
    console.error('Failed to upload dropped files:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.upload.failed'))
  } finally {
    isUploading.value = false
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
  const setupObserver = async () => {
    await nextTick()
    if (loadMoreObserver) {
      loadMoreObserver.disconnect()
      loadMoreObserver = null
    }
    loadMoreObserver = new IntersectionObserver(
      (entries) => {
        const entry = entries[0]
        if (!entry?.isIntersecting) return
        void loadMore()
      },
      {
        root: scrollContainerRef.value,
        rootMargin: '200px',
        threshold: 0,
      }
    )
    if (loadMoreSentinelRef.value) {
      loadMoreObserver.observe(loadMoreSentinelRef.value)
    }
  }

  // sentinel 在列表非空时才渲染，所以需要监听 ref 变化
  watch([scrollContainerRef, loadMoreSentinelRef], setupObserver, { immediate: true })

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

  // 上传进度（用于大批量上传时的即时反馈）
  unsubscribeUploadProgress = Events.On(
    'document:upload_progress',
    (event: { data: { library_id: number; total: number; done: number } }) => {
      const p = event.data
      if (p.library_id !== props.library?.id) return
      uploadTotal.value = p.total
      uploadDone.value = p.done
      if (p.total > 0 && p.done > 0 && p.done <= p.total) {
        isUploading.value = p.done < p.total
      }
    }
  )

  // 监听 Wails 原生文件拖拽事件
  unsubscribeFileDrop = Events.On(
    'filedrop:files',
    (event: { data: { files: string[] } }) => {
      const files = event.data?.files
      if (files && files.length > 0) {
        handleFileDrop(files)
      }
    }
  )

  // 单个文档已入库事件（可用于小批量即时显示；大批量仍以 resetAndLoad 为主）
  unsubscribeUploaded = Events.On('document:uploaded', (event: { data: BackendDocument }) => {
    const doc = event.data
    if (!doc || doc.library_id !== props.library?.id) return
    if (searchQuery.value.trim()) return
    // 仅在当前列表较少时追加，避免 500 次 DOM 更新
    if (documents.value.length >= PAGE_SIZE) return
    const converted = convertDocument(doc)
    const existingIndex = documents.value.findIndex((d) => d.id === converted.id)
    if (existingIndex >= 0) {
      documents.value[existingIndex] = converted
      return
    }
    documents.value.unshift(converted)
    documents.value.sort((a, b) =>
      sortBy.value === 'created_asc' ? a.id - b.id : b.id - a.id
    )
    if (documents.value.length > PAGE_SIZE) {
      documents.value.length = PAGE_SIZE
    }
    beforeID.value = documents.value.length ? documents.value[documents.value.length - 1].id : 0
  })
})

onUnmounted(() => {
  if (loadMoreObserver) {
    loadMoreObserver.disconnect()
    loadMoreObserver = null
  }
  if (unsubscribeProgress) {
    unsubscribeProgress()
  }
  if (unsubscribeThumbnail) {
    unsubscribeThumbnail()
  }
  if (unsubscribeUploadProgress) {
    unsubscribeUploadProgress()
  }
  if (unsubscribeUploaded) {
    unsubscribeUploaded()
  }
  if (unsubscribeFileDrop) {
    unsubscribeFileDrop()
  }
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }
})
</script>

<template>
  <div class="relative flex h-full flex-col" data-file-drop-target>
    <!-- 头部区域 -->
    <div class="flex h-12 items-center justify-between px-4">
      <h2 class="text-base font-medium text-foreground">{{ library.name }}</h2>
      <div class="flex items-center gap-1.5">
        <!-- 搜索框 -->
        <div class="relative w-40">
          <Search
            class="absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground/50"
          />
          <Input
            v-model="searchQuery"
            type="text"
            :placeholder="t('knowledge.content.searchPlaceholder')"
            class="h-6 pr-7 text-xs placeholder:text-muted-foreground/40"
          />
        </div>
        <!-- 排序按钮 -->
        <Button
          variant="ghost"
          size="icon"
          class="size-6"
          :title="
            sortBy === 'created_desc'
              ? t('knowledge.content.sort.createdDesc')
              : t('knowledge.content.sort.createdAsc')
          "
          @click="toggleSort"
        >
          <ArrowDownNarrowWide
            v-if="sortBy === 'created_desc'"
            class="size-4 text-muted-foreground"
          />
          <ArrowUpNarrowWide v-else class="size-4 text-muted-foreground" />
        </Button>
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

    <!-- 上传进度条（大批量上传时避免“卡住”的感觉） -->
    <div v-if="isUploading" class="px-4 pb-2">
      <div class="flex items-center justify-between text-xs text-muted-foreground">
        <span>{{
          t('knowledge.content.upload.uploading', { done: uploadDone, total: uploadTotal })
        }}</span>
        <span v-if="uploadTotal > 0">{{ Math.floor((uploadDone / uploadTotal) * 100) }}%</span>
      </div>
      <div class="mt-1 h-1 w-full overflow-hidden rounded bg-muted">
        <div
          class="h-full bg-foreground/30 transition-[width]"
          :style="{
            width:
              uploadTotal > 0
                ? `${Math.min(100, Math.floor((uploadDone / uploadTotal) * 100))}%`
                : '0%',
          }"
        />
      </div>
    </div>

    <!-- 内容区域 -->
    <div ref="scrollContainerRef" class="flex-1 overflow-auto p-4">
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
          <p class="mt-1 text-xs text-muted-foreground/70">
            {{ t('knowledge.content.empty.desc') }}
          </p>
        </div>
        <Button variant="outline" size="sm" class="gap-1.5" @click="handleAddDocument">
          <Plus class="size-4" />
          {{ t('knowledge.content.addDocument') }}
        </Button>
      </div>

      <!-- 文档网格 -->
      <div v-else>
        <div
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

        <div class="mt-4 flex items-center justify-center">
          <div v-if="isLoadingMore" class="text-xs text-muted-foreground">
            {{ t('knowledge.loading') }}
          </div>
          <div v-else-if="!hasMore" class="text-xs text-muted-foreground/60">
            {{ t('knowledge.content.noMore') }}
          </div>
        </div>

        <!-- sentinel for infinite scroll -->
        <div ref="loadMoreSentinelRef" class="h-1 w-full" />
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

    <!-- Drag-and-drop overlay (shown when Wails adds .file-drop-target-active) -->
    <div class="drop-overlay pointer-events-none">
      <div class="flex flex-col items-center gap-2">
        <Upload class="size-10 text-primary" />
        <p class="text-sm font-medium text-primary">
          {{ t('knowledge.content.drop.hint') }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ t('knowledge.content.drop.formats') }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Drag overlay: hidden by default, shown when Wails adds file-drop-target-active */
.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 50;
  display: none;
  align-items: center;
  justify-content: center;
  border-radius: inherit;
  border: 2px dashed hsl(var(--primary) / 0.5);
  background: hsl(var(--background) / 0.92);
  backdrop-filter: blur(4px);
  transition: opacity 0.15s ease;
}

/* When Wails detects a file drag over the drop target */
[data-file-drop-target].file-drop-target-active .drop-overlay {
  display: flex;
}
</style>
