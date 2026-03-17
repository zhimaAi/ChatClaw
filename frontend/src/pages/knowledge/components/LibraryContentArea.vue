<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialogs, Events } from '@wailsio/runtime'
import { Search, Upload, Plus, ArrowDownNarrowWide, ArrowUpNarrowWide, FolderPlus } from 'lucide-vue-next'
import IconUploadFile from '@/assets/icons/upload-file.svg'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
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
import FolderCard from './FolderCard.vue'
import RenameDocumentDialog from './RenameDocumentDialog.vue'
import MoveDocumentDialog from './MoveDocumentDialog.vue'
import DocumentDetailDialog from './DocumentDetailDialog.vue'
import { useNavigationStore } from '@/stores/navigation'
import CreateFolderDialog from './CreateFolderDialog.vue'
import RenameFolderDialog from './RenameFolderDialog.vue'
import DeleteFolderDialog from './DeleteFolderDialog.vue'
import MoveFolderDialog from './MoveFolderDialog.vue'
import type { Document, DocumentStatus } from './DocumentCard.vue'
import type { Library } from '@bindings/chatclaw/internal/services/library'
import {
  LibraryService,
  type Folder,
} from '@bindings/chatclaw/internal/services/library'
import {
  DocumentService,
  type Document as BackendDocument,
} from '@bindings/chatclaw/internal/services/document'
import { useAppStore } from '@/stores'

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

interface BrowserUploadFile {
  file_name: string
  base64_data: string
}

const props = defineProps<{
  library: Library
  selectedFolderId?: number | null
}>()

const emit = defineEmits<{
  'folder-selected': [folderId: number | null]
  'folder-created': []
  'folder-updated': []
  'folder-deleted': []
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()
const appStore = useAppStore()

const searchQuery = ref('')
const sortBy = ref<'created_desc' | 'created_asc'>('created_desc')
const deleteDialogOpen = ref(false)
const documentToDelete = ref<Document | null>(null)
const renameDialogOpen = ref(false)
const documentToRename = ref<Document | null>(null)
const moveDocumentDialogOpen = ref(false)
const documentToMove = ref<Document | null>(null)
const documentDetailDialogOpen = ref(false)
const documentToDetail = ref<Document | null>(null)
const documents = ref<Document[]>([])
const isLoading = ref(false)
const isLoadingMore = ref(false)
const hasMore = ref(true)
const beforeID = ref<number>(0)
const PAGE_SIZE = 100
let loadToken = 0

// Folder related state
const folders = ref<Folder[]>([])
// null = 全部, -1 = 未分组, >0 = 文件夹ID
const activeFolderId = ref<number | null>(null)
const createFolderDialogOpen = ref(false)
const renameFolderDialogOpen = ref(false)
const deleteFolderDialogOpen = ref(false)
const moveFolderDialogOpen = ref(false)
const folderToRename = ref<Folder | null>(null)
const folderToDelete = ref<Folder | null>(null)
const folderToMove = ref<Folder | null>(null)

// 文件夹查找缓存 Map（优化性能，避免重复递归查找）
const folderMapCache = ref<Map<number, Folder>>(new Map())

// 构建文件夹 Map 缓存（一次性构建，避免重复递归）
const buildFolderMap = (folders: Folder[], map: Map<number, Folder> = new Map()): Map<number, Folder> => {
  for (const folder of folders) {
    map.set(folder.id, folder)
    if (folder.children && folder.children.length > 0) {
      buildFolderMap(folder.children, map)
    }
  }
  return map
}

// 监听文件夹变化，更新缓存
watch(folders, (newFolders) => {
  folderMapCache.value = buildFolderMap(newFolders)
}, { deep: true })

const isUploading = ref(false)
const uploadTotal = ref(0)
const uploadDone = ref(0)
const isDragOver = ref(false)
let unsubscribeUploadProgress: (() => void) | null = null
let unsubscribeUploaded: (() => void) | null = null
let unsubscribeFileDrop: (() => void) | null = null

const dropTargetRef = ref<HTMLElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
let dragDepth = 0

const supportedUploadExtensions = ['pdf', 'doc', 'docx', 'txt', 'md', 'csv', 'xlsx', 'html', 'htm', 'ofd']
const desktopUploadPattern = supportedUploadExtensions.map((ext) => `*.${ext}`).join(';')
const browserUploadAccept = supportedUploadExtensions.map((ext) => `.${ext}`).join(',')

type BrowserUploadService = {
  UploadBrowserDocuments: (input: {
    library_id: number
    files: BrowserUploadFile[]
    folder_id: number | null
  }) => Promise<BackendDocument[]>
}

const browserUploadService = DocumentService as typeof DocumentService & BrowserUploadService

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
    updatedAt: doc.updated_at,
    folderId: doc.folder_id,
    status,
    progress,
    errorMessage,
    thumbIcon: doc.thumb_icon || undefined,
    fileMissing: doc.file_missing || false,
  }
}

const loadFolders = async () => {
  if (!props.library?.id) return
  try {
    folders.value = await LibraryService.ListFolders(props.library.id)
    // 每次文件夹结构变化时，同步刷新统计信息，确保"X项"显示准确
    void loadFolderStats()
  } catch (error) {
    console.error('Failed to load folders:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  }
}

const resetAndLoad = async () => {
  if (!props.library?.id) return

  loadToken += 1
  documents.value = []
  beforeID.value = 0
  hasMore.value = true // Always allow the first request to go through
  await loadMore(loadToken)
  // 重置并重新加载列表后，同步刷新文件夹统计
  void loadFolderStats()
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
      // Root (null) should show only uncategorized (folder_id IS NULL), not documents from subfolders
      folder_id:
        activeFolderId.value === null ? -1 : activeFolderId.value === -1 ? -1 : activeFolderId.value,
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

// 监听知识库变化，重新加载文档和文件夹
watch(
  () => props.library?.id,
  () => {
    searchQuery.value = ''
    activeFolderId.value = props.selectedFolderId ?? null
    void loadFolders()
    resetAndLoad()
  },
  { immediate: true }
)

// 监听外部传入的 selectedFolderId
watch(
  () => props.selectedFolderId,
  (newId) => {
    activeFolderId.value = newId ?? null
  }
)

// 监听文件夹变化，重新加载文档
watch(activeFolderId, () => {
  // If a folder is selected, load documents in that folder.
  // If root (null) is selected, show root folders and uncategorized documents only.
  resetAndLoad()
})

// 使用缓存 Map 快速查找文件夹（O(1) 时间复杂度，替代递归查找）
const findFolderById = (id: number): Folder | null => {
  return folderMapCache.value.get(id) || null
}

// 计算是否正在搜索
const isSearching = computed(() => {
  return searchQuery.value.trim().length > 0
})

// 计算要显示的文件夹列表
const displayFolders = computed(() => {
  // 如果 activeFolderId 为 null，显示根文件夹
  if (activeFolderId.value === null) {
    return folders.value
  }
  // 如果 activeFolderId 不为 null，显示当前文件夹的子文件夹
  if (activeFolderId.value > 0) {
    const currentFolder = findFolderById(activeFolderId.value)
    return currentFolder?.children || []
  }
  // activeFolderId 为 -1（未分组）时，不显示文件夹
  return []
})

// 构建面包屑路径数组
const breadcrumbPath = computed(() => {
  if (!props.library?.name) return []

  const path: Array<{ name: string; id: number | null }> = [
    { name: props.library.name, id: null }
  ]

  // 根目录或未分组：仅显示知识库名
  if (!activeFolderId.value || activeFolderId.value <= 0) {
    return path
  }

  const names: Array<{ name: string; id: number }> = []
  let cursorId: number | null = activeFolderId.value

  while (cursorId && cursorId > 0) {
    const folder = folderMapCache.value.get(cursorId)
    if (!folder) break
    names.push({ name: folder.name, id: folder.id })
    const parentId = (folder.parent_id as unknown as number | null) ?? null
    cursorId = parentId && parentId > 0 ? parentId : null
  }

  names.reverse()
  return [...path, ...names]
})

// 当前标题面包屑：知识库名 + 文件夹路径（用于 title 属性）
const currentBreadcrumbTitle = computed(() => {
  const path = breadcrumbPath.value
  if (path.length === 0) return ''
  if (path.length === 1) return path[0].name
  return path.map(p => p.name).join(' / ')
})

// 计算要显示的面包屑项（路径过长时，只显示首尾 + 倒数两级，中间用省略号）
const visibleBreadcrumbs = computed(() => {
  const path = breadcrumbPath.value
  if (path.length <= 4) {
    // 路径不长，全部显示
    return path.map((item, idx) => ({ ...item, visible: true, index: idx, isEllipsis: false }))
  }
  // 路径过长：显示首个（库名）+ 省略号 + 倒数第二级 + 最后一级
  const result: Array<{ name: string; id: number | null; visible: boolean; index: number; isEllipsis: boolean }> = []
  const lastIndex = path.length - 1
  const secondLastIndex = path.length - 2

  // 首个：库名
  result.push({ ...path[0], visible: true, index: 0, isEllipsis: false })
  // 中间省略号（不可点击）
  result.push({ name: '...', id: null, visible: true, index: -1, isEllipsis: true })
  // 倒数第二级（方便返回上级）
  result.push({
    ...path[secondLastIndex],
    visible: true,
    index: secondLastIndex,
    isEllipsis: false,
  })
  // 最后一级：当前文件夹
  result.push({
    ...path[lastIndex],
    visible: true,
    index: lastIndex,
    isEllipsis: false,
  })
  return result
})

// 后端提供的文件夹统计信息（文档数量 & 最近更新时间）
const folderStatsMap = ref<
  Map<
    number,
    {
      docCount: number
      latestDocUpdatedAt?: string
    }
  >
>(new Map())

const loadFolderStats = async () => {
  if (!props.library?.id) return
  try {
    const stats = await LibraryService.GetFolderStats(props.library.id)
    const map = new Map<
      number,
      {
        docCount: number
        latestDocUpdatedAt?: string
      }
    >()

    for (const item of stats) {
      if (!item.folder_id || item.folder_id <= 0) continue
      map.set(item.folder_id, {
        docCount: item.doc_count ?? 0,
        latestDocUpdatedAt: item.latest_doc_updated_at as unknown as string | undefined,
      })
    }

    folderStatsMap.value = map
  } catch (error) {
    console.error('Failed to load folder stats:', error)
  }
}

const formatDate = (dateStr: string | null | undefined) => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}/${month}/${day}`
}

const getFolderItemCount = (folder: Folder): number => {
  const subFolderCount = folder.children?.length ?? 0
  const stats = folderStatsMap.value.get(folder.id)
  const docCount = stats?.docCount ?? 0
  return subFolderCount + docCount
}

const getFolderLatestUpdatedAt = (folder: Folder): string | undefined => {
  // Prefer the latest between folder.updated_at and its documents' updated_at
  const stats = folderStatsMap.value.get(folder.id)
  const candidates: string[] = []
  if (folder.updated_at) {
    candidates.push(String(folder.updated_at as unknown as string))
  }
  if (stats?.latestDocUpdatedAt) {
    candidates.push(stats.latestDocUpdatedAt)
  }
  if (candidates.length === 0) return undefined

  let latest: string | undefined
  let latestTs = 0
  for (const value of candidates) {
    const ts = new Date(value).getTime()
    if (Number.isNaN(ts)) continue
    if (ts > latestTs) {
      latestTs = ts
      latest = value
    }
  }

  return latest ? formatDate(latest) : undefined
}

// 处理文件夹点击
const handleFolderClick = (folder: Folder) => {
  activeFolderId.value = folder.id
  // 通知父组件文件夹选择变化
  emit('folder-selected', folder.id)
}

// 处理文件夹重命名
const handleFolderRename = (folder: Folder) => {
  folderToRename.value = folder
  renameFolderDialogOpen.value = true
}

// 处理文件夹删除
const handleFolderDelete = (folder: Folder) => {
  folderToDelete.value = folder
  deleteFolderDialogOpen.value = true
}

// 处理文件夹移动
const handleFolderMove = (folder: Folder) => {
  folderToMove.value = folder
  moveFolderDialogOpen.value = true
}

const handleOpenCreateFolder = () => {
  createFolderDialogOpen.value = true
}

// 处理文件夹创建
const handleFolderCreated = (createdFolder: Folder) => {
  void loadFolders()
  emit('folder-created')
  // 刷新文档列表，以便显示新创建的文件夹（如果当前在文件夹内部）
  void resetAndLoad()
  // 如果创建的是子文件夹，自动选中新创建的文件夹
  if (createdFolder.parent_id) {
    activeFolderId.value = createdFolder.id
    emit('folder-selected', createdFolder.id)
  }
}

// 处理文件夹更新
const handleFolderUpdated = () => {
  void loadFolders()
  emit('folder-updated')
}

// 处理文件夹移动完成
const handleFolderMoved = () => {
  void loadFolders()
  emit('folder-updated')
  // 如果移动的是当前文件夹，需要更新 activeFolderId
  if (folderToMove.value && activeFolderId.value === folderToMove.value.id) {
    // 移动后，文件夹的 parent_id 会变化，但 id 不变，所以不需要特别处理
    // 但如果移动到根目录，可能需要刷新显示
    void resetAndLoad()
  }
}

// 处理文件夹删除完成
const handleFolderDeleted = () => {
  void loadFolders()
  const deletedFolderId = folderToDelete.value?.id
  if (deletedFolderId !== undefined && activeFolderId.value === deletedFolderId) {
    activeFolderId.value = null
    emit('folder-selected', null)
  }
  emit('folder-deleted')
}

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
  if (appStore.isServerMode) {
    if (fileInputRef.value) {
      fileInputRef.value.value = ''
      fileInputRef.value.click()
    }
    return
  }

  try {
    const result = await Dialogs.OpenFile({
      Title: t('knowledge.content.selectFile'),
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: true,
      Filters: [
        {
          DisplayName: t('knowledge.content.fileTypes.documents'),
          Pattern: desktopUploadPattern,
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

      // 上传文件，传入当前选中的文件夹 ID
      const folderId = activeFolderId.value && activeFolderId.value > 0 ? activeFolderId.value : null
      const uploaded = await DocumentService.UploadDocuments({
        library_id: props.library.id,
        file_paths: result,
        folder_id: folderId,
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

const readFileAsBase64 = (file: File): Promise<string> => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result
      if (typeof result !== 'string') {
        reject(new Error('invalid file data'))
        return
      }
      const commaIndex = result.indexOf(',')
      resolve(commaIndex >= 0 ? result.slice(commaIndex + 1) : result)
    }
    reader.onerror = () => reject(reader.error || new Error('failed to read file'))
    reader.readAsDataURL(file)
  })
}

const uploadBrowserFiles = async (files: FileList | File[]) => {
  if (!props.library?.id) return
  if (isUploading.value) return

  const fileArray = Array.from(files)
  if (fileArray.length === 0) return

  try {
    isUploading.value = true
    uploadTotal.value = fileArray.length
    uploadDone.value = 0
    await nextTick()

    const uploadFiles: BrowserUploadFile[] = []
    for (const file of fileArray) {
      uploadFiles.push({
        file_name: file.name,
        base64_data: await readFileAsBase64(file),
      })
    }

    const folderId = activeFolderId.value && activeFolderId.value > 0 ? activeFolderId.value : null
    const uploaded = await browserUploadService.UploadBrowserDocuments({
      library_id: props.library.id,
      files: uploadFiles,
      folder_id: folderId,
    })

    await resetAndLoad()
    toast.success(t('knowledge.content.upload.count', { count: uploaded.length }))
  } catch (error) {
    console.error('Failed to upload browser documents:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.upload.failed'))
  } finally {
    isUploading.value = false
  }
}

const handleBrowserFileInputChange = (event: Event) => {
  const input = event.target as HTMLInputElement | null
  const files = input?.files
  if (files && files.length > 0) {
    void uploadBrowserFiles(files)
  }
  if (input) {
    input.value = ''
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

    // 上传文件，传入当前选中的文件夹 ID
    const folderId = activeFolderId.value && activeFolderId.value > 0 ? activeFolderId.value : null
    const uploaded = await DocumentService.UploadDocuments({
      library_id: props.library.id,
      file_paths: filePaths,
      folder_id: folderId,
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

const handleMoveToFolder = (doc: Document) => {
  documentToMove.value = doc
  moveDocumentDialogOpen.value = true
}

const handleDetail = (doc: Document) => {
  documentToDetail.value = doc
  documentDetailDialogOpen.value = true
}

const handleView = (doc: Document) => {
  // Open document in a new tab instead of dialog
  navigationStore.openDocumentViewer(doc.id, doc.name, doc.thumbIcon)
}

// 处理跳转到文档所在文件夹
const handleNavigateToFolder = (doc: Document) => {
  if (doc.folderId && doc.folderId > 0) {
    activeFolderId.value = doc.folderId
    emit('folder-selected', doc.folderId)
    // 清除搜索，显示文件夹内容
    searchQuery.value = ''
  } else {
    // 跳转到未分组
    activeFolderId.value = -1
    emit('folder-selected', -1)
    searchQuery.value = ''
  }
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
    // 删除文档后刷新文件夹统计，保持"X项"准确
    void loadFolderStats()

    toast.success(t('knowledge.content.delete.success'))
    // 删除成功后关闭详情弹窗
    documentDetailDialogOpen.value = false
  } catch (error) {
    console.error('Failed to delete document:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.delete.failed'))
  } finally {
    deleteDialogOpen.value = false
    documentToDelete.value = null
  }
}

// 仅在拖拽文件时显示遮罩
const isFileDragEvent = (event: DragEvent) =>
  !!event.dataTransfer && Array.from(event.dataTransfer.types).includes('Files')

const resetDragState = () => {
  dragDepth = 0
  isDragOver.value = false
}

const handleDragEnter = (event: DragEvent) => {
  if (!isFileDragEvent(event)) return
  event.preventDefault()
  dragDepth += 1
  isDragOver.value = true
}

const handleDragOverEvent = (event: DragEvent) => {
  if (!isFileDragEvent(event)) return
  event.preventDefault()
}

const handleDragLeave = (event: DragEvent) => {
  if (!isFileDragEvent(event)) return
  event.preventDefault()
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) {
    isDragOver.value = false
  }
}

const handleDropEvent = (event: DragEvent) => {
  if (!isFileDragEvent(event)) return
  event.preventDefault()
  if (appStore.isServerMode) {
    const files = event.dataTransfer?.files
    if (files && files.length > 0) {
      void uploadBrowserFiles(files)
    }
  }
  resetDragState()
}

const handleGlobalDragEnd = () => {
  resetDragState()
}

const addDragListeners = (el: HTMLElement | null) => {
  if (!el) return
  el.addEventListener('dragenter', handleDragEnter)
  el.addEventListener('dragover', handleDragOverEvent)
  el.addEventListener('dragleave', handleDragLeave)
  el.addEventListener('drop', handleDropEvent)
}

const removeDragListeners = (el: HTMLElement | null) => {
  if (!el) return
  el.removeEventListener('dragenter', handleDragEnter)
  el.removeEventListener('dragover', handleDragOverEvent)
  el.removeEventListener('dragleave', handleDragLeave)
  el.removeEventListener('drop', handleDropEvent)
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

  // 监听拖拽目标元素变化，绑定/解绑原生拖拽事件
  watch(
    () => dropTargetRef.value,
    (el, prevEl) => {
      if (prevEl) {
        removeDragListeners(prevEl)
      }
      if (el) {
        addDragListeners(el)
      }
    },
    { immediate: true }
  )

  window.addEventListener('dragend', handleGlobalDragEnd)

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
    // Sync opened document tab icon (if any) to match the card thumbnail.
    navigationStore.updateDocumentTabIconByDocumentId(
      thumbnail.document_id,
      thumbnail.thumb_icon || undefined
    )
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
  if (appStore.isGUIMode) {
    unsubscribeFileDrop = Events.On(
      'filedrop:files',
      (event: { data: { files: string[] } }) => {
        const files = event.data?.files
        if (files && files.length > 0) {
          handleFileDrop(files)
        }
      }
    )
  }

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

  window.removeEventListener('dragend', handleGlobalDragEnd)
  if (dropTargetRef.value) {
    removeDragListeners(dropTargetRef.value)
  }
})
</script>

<template>
  <div
    ref="dropTargetRef"
    class="relative flex min-h-0 flex-1 flex-col"
    data-file-drop-target
  >
    <input
      ref="fileInputRef"
      type="file"
      multiple
      class="hidden"
      :accept="browserUploadAccept"
      @change="handleBrowserFileInputChange"
    >
    <!-- 头部区域 -->
    <div class="flex h-12 items-center justify-between gap-4 px-4">
      <div class="flex min-w-0 flex-1 items-center gap-3">
        <!-- 面包屑导航（Windows 资源管理器样式：路径过长时中间省略） -->
        <nav
          class="flex min-w-0 flex-1 items-center gap-1 overflow-hidden text-base font-medium text-foreground"
          :title="currentBreadcrumbTitle"
        >
          <template v-for="(item, idx) in visibleBreadcrumbs" :key="`${item.id ?? 'root'}-${idx}`">
            <span v-if="Number(idx) > 0 && !item.isEllipsis" class="shrink-0 px-1 text-muted-foreground/60">/</span>
            <button
              v-if="!item.isEllipsis"
              type="button"
              class="shrink-0 truncate rounded px-1 py-0.5 text-sm transition-colors hover:bg-accent/50 hover:text-foreground"
              :class="item.index === breadcrumbPath.length - 1 ? 'font-medium' : 'text-muted-foreground'"
              :title="item.name"
              @click="
                () => {
                  // id=null 表示知识库根目录，点击返回根目录
                  activeFolderId = item.id
                  emit('folder-selected', item.id)
                }
              "
            >
              {{ item.name }}
            </button>
            <span
              v-else
              class="shrink-0 px-1 text-muted-foreground/60"
              :title="currentBreadcrumbTitle"
            >
              {{ item.name }}
            </span>
          </template>
        </nav>
      </div>
      <div class="flex shrink-0 items-center gap-1.5">
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
        <!-- 添加文件 / 文件夹菜单 -->
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button
              variant="ghost"
              size="icon"
              class="size-6"
              :title="t('knowledge.content.addDocument')"
            >
              <Plus class="size-4 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" class="w-36">
            <DropdownMenuItem class="gap-2" @select="handleAddDocument">
              <IconUploadFile class="size-4 text-muted-foreground" />
              <span>{{ t('knowledge.content.addDocument') }}</span>
            </DropdownMenuItem>
            <DropdownMenuItem class="gap-2" @select="handleOpenCreateFolder">
              <FolderPlus class="size-4 text-muted-foreground" />
              <span>{{ t('knowledge.folder.create') }}</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
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
    <ContextMenu>
      <ContextMenuTrigger as-child>
        <div ref="scrollContainerRef" class="flex-1 overflow-auto p-4">
          <!-- 加载中 -->
          <div v-if="isLoading" class="flex h-full items-center justify-center">
            <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
          </div>

          <!-- 空状态（没有文件也没有文件夹时才显示） -->
          <div
            v-else-if="filteredDocuments.length === 0 && (displayFolders.length === 0 || searchQuery.trim())"
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

          <!-- 文件夹和文档网格 -->
          <div v-else>
            <div
              class="grid auto-rows-max gap-4"
              style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
            >
              <!-- 文件夹卡片（仅在显示"全部"时且未搜索时显示） -->
              <FolderCard
                v-for="folder in displayFolders"
                :key="`folder-${folder.id}`"
                :folder="folder"
                :document-count="getFolderItemCount(folder)"
                :latest-updated-at="getFolderLatestUpdatedAt(folder)"
                v-show="!searchQuery.trim()"
                @click="handleFolderClick"
                @rename="handleFolderRename"
                @delete="handleFolderDelete"
                @move="handleFolderMove"
                @contextmenu.stop
              />
              <!-- 文档卡片 -->
              <DocumentCard
                v-for="doc in filteredDocuments"
                :key="doc.id"
                :document="doc"
                :is-searching="isSearching"
                @rename="handleRename"
                @relearn="handleRelearn"
                @delete="handleOpenDelete"
                @move-to-folder="handleMoveToFolder"
                @detail="handleDetail"
                @view="handleView"
                @navigate-to-folder="handleNavigateToFolder"
                @contextmenu.stop
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
      </ContextMenuTrigger>
      <ContextMenuContent class="w-auto min-w-max">
        <ContextMenuItem class="gap-2 whitespace-nowrap" @select="handleOpenCreateFolder">
          <FolderPlus class="size-4 text-muted-foreground" />
          <span>{{ t('knowledge.folder.create') }}</span>
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>

    <!-- 重命名对话框 -->
    <RenameDocumentDialog
      v-model:open="renameDialogOpen"
      :document="documentToRename"
      @confirm="confirmRename"
    />

    <!-- 移动到文件夹对话框 -->
    <MoveDocumentDialog
      v-model:open="moveDocumentDialogOpen"
      :document="documentToMove"
      :folders="folders"
      @moved="
        () => {
          // 如果当前过滤条件与目标文件夹不符，从列表中移除
          resetAndLoad()
        }
      "
    />

    <!-- 文档详情对话框 -->
    <DocumentDetailDialog
      v-model:open="documentDetailDialogOpen"
      :document="documentToDetail"
      :folders="folders"
      @relearn="handleRelearn"
      @move-to-folder="handleMoveToFolder"
      @rename="handleRename"
      @delete="handleOpenDelete"
    />

    <!-- 文件夹管理对话框 -->
    <CreateFolderDialog
      v-model:open="createFolderDialogOpen"
      :library-id="library.id"
      :folders="folders"
      :default-parent-id="activeFolderId"
      @created="handleFolderCreated"
    />
    <RenameFolderDialog
      v-model:open="renameFolderDialogOpen"
      :folder="folderToRename"
      @updated="handleFolderUpdated"
    />
    <DeleteFolderDialog
      v-model:open="deleteFolderDialogOpen"
      :folder="folderToDelete"
      @deleted="handleFolderDeleted"
    />
    <MoveFolderDialog
      v-model:open="moveFolderDialogOpen"
      :folder="folderToMove"
      :folders="folders"
      @moved="handleFolderMoved"
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
            class="bg-foreground text-background hover:bg-foreground/90"
            @click.prevent="confirmDelete"
          >
            {{ t('knowledge.content.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- Drag-and-drop overlay -->
    <div v-show="isDragOver" class="drop-overlay pointer-events-none">
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
/* Drag overlay: visibility is controlled by Vue via v-show */
.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: inherit;
  border: 2px dashed hsl(var(--primary) / 0.5);
  background: hsl(var(--background) / 0.92);
  backdrop-filter: blur(4px);
  transition: opacity 0.15s ease;
}
</style>
