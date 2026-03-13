<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, MoreHorizontal, Settings, FileText, Folder as FolderIcon } from 'lucide-vue-next'
import IconKnowledge from '@/assets/icons/knowledge.svg'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 */
const props = defineProps<{
  tabId: string
}>()
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useNavigationStore, useSettingsStore } from '@/stores'
import CreateLibraryDialog from './components/CreateLibraryDialog.vue'
import EmbeddingSettingsDialog from './components/EmbeddingSettingsDialog.vue'
import RenameLibraryDialog from './components/RenameLibraryDialog.vue'
import EditLibraryDialog from './components/EditLibraryDialog.vue'
import LibraryContentArea from './components/LibraryContentArea.vue'
import FolderTreeItem from './components/FolderTreeItem.vue'
import ChatInputArea from '@/pages/assistant/components/ChatInputArea.vue'
import IconRename from '@/assets/icons/library-rename.svg'
import IconLibSettings from '@/assets/icons/library-settings.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import IconPdf from '@/assets/icons/file-pdf.svg'
import IconWord from '@/assets/icons/file-word.svg'
import IconExcel from '@/assets/icons/file-excel.svg'
import IconText from '@/assets/icons/file-text.svg'
import IconMarkdown from '@/assets/icons/file-markdown.svg'
import IconHtml from '@/assets/icons/file-html.svg'
import IconCsv from '@/assets/icons/file-csv.svg'
import IconOfd from '@/assets/icons/file-ofd.svg'
import IconSidebarCollapse from '@/assets/icons/sidebar-collapse.svg'
import IconSidebarExpand from '@/assets/icons/sidebar-expand.svg'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
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

import type { Library } from '@bindings/chatclaw/internal/services/library'
import { LibraryService, type Folder } from '@bindings/chatclaw/internal/services/library'
import {
  ChatWikiService,
  type LibraryFile as ChatWikiLibraryFile,
  type LibraryGroup as ChatWikiLibraryGroup,
  type LibraryParagraph as ChatWikiLibraryParagraph,
} from '@bindings/chatclaw/internal/services/chatwiki'
import { getBinding as getBindingCached, getLibraryListOnlyOpen as getLibraryListOnlyOpenCached } from '@/lib/chatwikiCache'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import { Book, BookOpen, ChevronRight, FileStack } from 'lucide-vue-next'
import { useAgents } from '@/pages/assistant/composables/useAgents'
import { useModelSelection } from '@/pages/assistant/composables/useModelSelection'
import { supportsMultimodal } from '@/composables/useMultimodal'
import { toast } from '@/components/ui/toast'

type LibraryTab = 'personal' | 'team'

const { t } = useI18n()
const navigationStore = useNavigationStore()
const settingsStore = useSettingsStore()

const activeTab = ref<LibraryTab>('personal')
const createDialogOpen = ref(false)
const embeddingSettingsOpen = ref(false)
const renameOpen = ref(false)
const editOpen = ref(false)
const deleteOpen = ref(false)
const actionLibrary = ref<Library | null>(null)

const libraries = ref<Library[]>([])
const loading = ref(false)
const selectedLibraryId = ref<number | null>(null)
const libraryFolders = ref<Map<number, Folder[]>>(new Map())
const expandedLibraries = ref<Set<number>>(new Set())
const expandedFolders = ref<Set<number>>(new Set())
// null = 根目录, -1 = 未分组, >0 = 文件夹ID
const selectedFolderId = ref<number | null>(null)
// Left sidebar collapsed state (narrow strip with icons only)
const sidebarCollapsed = ref(false)

// Chat input state for the bottom input area
const chatInput = ref('')
const enableThinking = ref(false)
const chatMode = ref('task')
const pendingImages = ref<PendingImage[]>([])

// Use composables for agent and model selection
const {
  agents,
  activeAgentId,
  loadAgents,
} = useAgents()

const {
  providersWithModels,
  selectedModelKey,
  hasModels,
  selectedModelInfo,
  loadModels,
  selectDefaultModel,
} = useModelSelection()

interface PendingImage {
  id: string
  file: File
  mimeType: string
  base64: string
  dataUrl: string
  fileName: string
  size: number
}

// Computed: active agent
const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

// Can send: must have input or images, agent, and model
const canSend = computed(() => {
  const hasContent = chatInput.value.trim() !== '' || pendingImages.value.length > 0
  return (
    !!activeAgentId.value &&
    hasContent &&
    !!selectedModelInfo.value
  )
})

// Reason why send is disabled
const sendDisabledReason = computed(() => {
  if (!activeAgentId.value) return t('assistant.placeholders.createAgentFirst')
  if (!selectedModelKey.value) return t('assistant.placeholders.selectModelFirst')
  const hasContent = chatInput.value.trim() !== '' || pendingImages.value.length > 0
  if (!hasContent) return t('assistant.placeholders.enterToSend')
  return ''
})

const selectedLibrary = computed(
  () => libraries.value.find((l) => l.id === selectedLibraryId.value) || null
)
interface TeamLibraryItem {
  id: string
  name: string
  intro: string
  type: string
  type_name: string
  chat_claw_switch_status: number
}
const teamBindingChecked = ref(false)
const teamBound = ref(false)
const teamLibraries = ref<TeamLibraryItem[]>([])
const teamLibrariesLoading = ref(false)
const teamLibraryTab = ref(0)
const selectedTeamLibraryId = ref<string | null>(null)
const selectedTeamLibrary = computed(
  () => teamLibraries.value.find((l) => l.id === selectedTeamLibraryId.value) || null
)
const TEAM_ALL_GROUP_ID = '-1'
const TEAM_PAGE_SIZE = 10
const TEAM_LOAD_MORE_THRESHOLD = 360
const TEAM_SKELETON_COUNT = 6
const teamLibraryGroups = ref<ChatWikiLibraryGroup[]>([])
const teamLibraryFiles = ref<ChatWikiLibraryFile[]>([])
const teamUngroupedFiles = ref<ChatWikiLibraryFile[]>([])
const teamLibraryGroupsLoading = ref(false)
const teamLibraryFilesLoading = ref(false)
const teamLibraryFilesLoadingMore = ref(false)
const teamUngroupedFilesLoading = ref(false)
const teamUngroupedFilesLoadingMore = ref(false)
const teamLibraryFilesPage = ref(1)
const teamUngroupedFilesPage = ref(1)
const teamLibraryFilesHasMore = ref(true)
const teamUngroupedFilesHasMore = ref(true)
const teamNormalSearchInput = ref('')
const teamNormalSearchKeyword = ref('')
const teamQAParagraphs = ref<ChatWikiLibraryParagraph[]>([])
const teamQAParagraphsLoading = ref(false)
const teamQAParagraphsLoadingMore = ref(false)
const teamQAParagraphsPage = ref(1)
const teamQAParagraphsHasMore = ref(true)
const teamQAParagraphTotal = ref(-1)
const teamNormalTotal = ref(-1)
const teamWechatTotal = ref(-1)
const teamQASearchInput = ref('')
const teamQASearchKeyword = ref('')
const selectedTeamGroupId = ref<string>(TEAM_ALL_GROUP_ID)
const teamNormalPage = ref<'groups' | 'files'>('groups')
const teamScrollContainerRef = ref<HTMLElement | null>(null)
let teamNormalRequestID = 0
let teamScrollRafID: number | null = null
const teamGroupCards = computed(() => teamLibraryGroups.value)
const selectedTeamGroupName = computed(
  () => teamLibraryGroups.value.find((group) => group.id === selectedTeamGroupId.value)?.name || ''
)


const getTeamFileExtension = (file: ChatWikiLibraryFile) => {
  const ext = String(file.extension || '').trim().toLowerCase()
  if (ext) return ext
  const name = String(file.name || '')
  const idx = name.lastIndexOf('.')
  if (idx < 0 || idx === name.length - 1) return ''
  return name.slice(idx + 1).trim().toLowerCase()
}

const getTeamFileIcon = (file: ChatWikiLibraryFile) => {
  const ext = getTeamFileExtension(file)
  switch (ext) {
    case 'pdf':
      return IconPdf
    case 'doc':
    case 'docx':
      return IconWord
    case 'xls':
    case 'xlsx':
      return IconExcel
    case 'txt':
      return IconText
    case 'md':
    case 'markdown':
      return IconMarkdown
    case 'html':
    case 'htm':
      return IconHtml
    case 'csv':
      return IconCsv
    case 'ofd':
      return IconOfd
    default:
      return FileText
  }
}

const formatTeamFileDate = (dateStr: string) => {
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}/${month}/${day}`
}

const getTeamFileStatusLabel = (status: number) => {
  if (status === 3) return t('knowledge.content.status.failed')
  if (status === 2) return ''
  if (status === 1) return t('knowledge.content.status.learning')
  return t('knowledge.content.status.pending')
}

const getTeamFileThumbUrl = (file: ChatWikiLibraryFile) => {
  const thumbPath = String(file.thumb_path || '').trim()
  return thumbPath
}

const teamThumbLoadFailedKeys = ref<Set<string>>(new Set())

const getTeamFileThumbKey = (file: ChatWikiLibraryFile) => {
  const thumbPath = getTeamFileThumbUrl(file)
  return `${file.id}::${thumbPath}`
}

const canShowTeamFileThumb = (file: ChatWikiLibraryFile) => {
  const thumbPath = getTeamFileThumbUrl(file)
  if (!thumbPath) return false
  return !teamThumbLoadFailedKeys.value.has(getTeamFileThumbKey(file))
}

const handleTeamFileThumbError = (file: ChatWikiLibraryFile) => {
  const key = getTeamFileThumbKey(file)
  if (teamThumbLoadFailedKeys.value.has(key)) return
  const next = new Set(teamThumbLoadFailedKeys.value)
  next.add(key)
  teamThumbLoadFailedKeys.value = next
}

const clearTeamNormalContent = () => {
  teamLibraryGroups.value = []
  teamLibraryFiles.value = []
  teamUngroupedFiles.value = []
  teamLibraryFilesPage.value = 1
  teamUngroupedFilesPage.value = 1
  teamLibraryFilesHasMore.value = true
  teamUngroupedFilesHasMore.value = true
  teamLibraryFilesLoadingMore.value = false
  teamUngroupedFilesLoadingMore.value = false
  selectedTeamGroupId.value = TEAM_ALL_GROUP_ID
  teamNormalPage.value = teamLibraryTab.value === 0 ? 'groups' : 'files'
  teamNormalTotal.value = -1
  teamWechatTotal.value = -1
}

const applyTeamNormalSearch = () => {
  const keyword = teamNormalSearchInput.value.trim()
  if (keyword === teamNormalSearchKeyword.value) return
  teamNormalSearchKeyword.value = keyword
  if (!selectedTeamLibraryId.value) return
  const requestID = ++teamNormalRequestID
  if (teamLibraryTab.value === 0 && teamNormalPage.value === 'groups') {
    void loadTeamUngroupedFiles(selectedTeamLibraryId.value, requestID)
    return
  }
  const groupID = teamLibraryTab.value === 0 ? selectedTeamGroupId.value : ''
  void loadTeamLibraryFiles(selectedTeamLibraryId.value, requestID, false, groupID)
}

const clearTeamQAContent = () => {
  teamQAParagraphs.value = []
  teamQAParagraphsPage.value = 1
  teamQAParagraphsHasMore.value = true
  teamQAParagraphsLoading.value = false
  teamQAParagraphsLoadingMore.value = false
  teamQAParagraphTotal.value = -1
}

// Whether the library list is empty (loaded & no personal libraries)
const isLibraryEmpty = computed(
  () => !loading.value && activeTab.value === 'personal' && libraries.value.length === 0
)
const isPersonalTab = computed(() => activeTab.value === 'personal')

const loadLibraries = async () => {
  loading.value = true
  try {
    const list = await LibraryService.ListLibraries()
    libraries.value = list || []
    if (selectedLibraryId.value == null && libraries.value.length > 0) {
      selectedLibraryId.value = libraries.value[0].id
      // 默认展示根目录（文件夹 + 未分组文件）
      selectedFolderId.value = null
      // 自动展开第一个知识库
      expandedLibraries.value.add(libraries.value[0].id)
      // 加载第一个知识库的文件夹
      await loadFoldersForLibrary(libraries.value[0].id)
    }
  } catch (error) {
    console.error('Failed to load libraries:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    loading.value = false
  }
}

const loadFoldersForLibrary = async (libraryId: number, force = false) => {
  if (!force && libraryFolders.value.has(libraryId)) return
  try {
    const folders = await LibraryService.ListFolders(libraryId)
    libraryFolders.value.set(libraryId, folders)
  } catch (error) {
    console.error('Failed to load folders:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  }
}

const toggleLibraryExpanded = async (libraryId: number) => {
  if (expandedLibraries.value.has(libraryId)) {
    expandedLibraries.value.delete(libraryId)
  } else {
    expandedLibraries.value.add(libraryId)
    await loadFoldersForLibrary(libraryId)
  }
}

const toggleFolderExpanded = (folderId: number) => {
  if (expandedFolders.value.has(folderId)) {
    expandedFolders.value.delete(folderId)
  } else {
    expandedFolders.value.add(folderId)
  }
}

const handleFolderClick = (folderId: number | -1, libraryId: number) => {
  // 切换文件夹时，始终同步当前知识库，避免出现“文件夹属于库 B，但右侧仍显示库 A”的情况
  selectedLibraryId.value = libraryId
  // -1 表示"未分组"
  selectedFolderId.value = folderId === -1 ? -1 : folderId
}

const handleLibraryClick = async (libraryId: number) => {
  selectedLibraryId.value = libraryId
  // 切换知识库时默认展示该库根目录（文件夹 + 未分组文件）
  selectedFolderId.value = null
  if (!expandedLibraries.value.has(libraryId)) {
    expandedLibraries.value.add(libraryId)
    await loadFoldersForLibrary(libraryId)
  }
}

const handleCollapsedPersonalLibraryClick = async (libraryId: number) => {
  selectedLibraryId.value = libraryId
  selectedFolderId.value = null
  if (!expandedLibraries.value.has(libraryId)) {
    expandedLibraries.value.add(libraryId)
  }
  sidebarCollapsed.value = false
  await loadFoldersForLibrary(libraryId)
}

const handleCollapsedTeamLibraryClick = (libraryId: string) => {
  selectedTeamLibraryId.value = libraryId
  sidebarCollapsed.value = false
}

// 处理文件夹选择
const handleFolderSelected = (folderId: number | null) => {
  selectedFolderId.value = folderId
  // 如果选择的是文件夹，确保父文件夹在侧边栏中展开
  if (folderId && folderId > 0 && selectedLibrary.value) {
    const folders = libraryFolders.value.get(selectedLibrary.value.id) || []
    const findFolder = (folders: Folder[], id: number): Folder | null => {
      for (const folder of folders) {
        if (folder.id === id) return folder
        if (folder.children) {
          const found = findFolder(folder.children, id)
          if (found) {
            // 确保父文件夹展开
            expandedFolders.value.add(folder.id)
            return found
          }
        }
      }
      return null
    }
    findFolder(folders, folderId)
  }
}

// 处理文件夹创建
const handleFolderCreated = () => {
  if (selectedLibrary.value) {
    void loadFoldersForLibrary(selectedLibrary.value.id, true)
  }
}

// 处理文件夹更新
const handleFolderUpdated = () => {
  if (selectedLibrary.value) {
    void loadFoldersForLibrary(selectedLibrary.value.id, true)
  }
}

// 处理文件夹删除
const handleFolderDeleted = () => {
  if (selectedLibrary.value) {
    void loadFoldersForLibrary(selectedLibrary.value.id, true)
  }
}

// 监听知识库删除，清理相关状态
watch(
  () => libraries.value.map((l) => l.id),
  (newIds) => {
    // 清理已删除知识库的文件夹数据
    for (const [libId] of libraryFolders.value) {
      if (!newIds.includes(libId)) {
        libraryFolders.value.delete(libId)
        expandedLibraries.value.delete(libId)
      }
    }
    // 如果当前选中的知识库被删除，重置选择
    if (selectedLibraryId.value && !newIds.includes(selectedLibraryId.value)) {
      selectedLibraryId.value = newIds[0] ?? null
      selectedFolderId.value = null
    }
  }
)

const ensureEmbeddingConfigured = async (): Promise<boolean> => {
  try {
    const [p, m] = await Promise.all([
      SettingsService.Get('embedding_provider_id'),
      SettingsService.Get('embedding_model_id'),
    ])
    return !!(p?.value?.trim() && m?.value?.trim())
  } catch (error) {
    console.error('Failed to read embedding settings:', error)
    return false
  }
}

const handleCreateClick = async () => {
  const ok = await ensureEmbeddingConfigured()
  if (!ok) {
    toast.error(t('knowledge.embeddingSettings.required'))
    embeddingSettingsOpen.value = true
    return
  }
  createDialogOpen.value = true
}

const handleEmbeddingSettingsClick = () => {
  embeddingSettingsOpen.value = true
}

const goToChatwikiBindingSettings = () => {
  settingsStore.setActiveMenu('chatwiki')
  navigationStore.navigateToModule('settings')
}

const loadTeamLibraries = async () => {
  teamLibrariesLoading.value = true
  try {
    const list = await getLibraryListOnlyOpenCached(teamLibraryTab.value)
    teamLibraries.value = (list || []).map((item: any) => ({
      id: String(item?.id ?? ''),
      name: String(item?.name ?? ''),
      intro: String(item?.intro ?? ''),
      type: String(item?.type ?? ''),
      type_name: String(item?.type_name ?? ''),
      chat_claw_switch_status: Number(item?.chat_claw_switch_status ?? 0),
    }))
    if (!selectedTeamLibraryId.value && teamLibraries.value.length > 0) {
      selectedTeamLibraryId.value = teamLibraries.value[0].id
    }
    if (
      selectedTeamLibraryId.value &&
      !teamLibraries.value.some((item) => item.id === selectedTeamLibraryId.value)
    ) {
      selectedTeamLibraryId.value = teamLibraries.value[0]?.id ?? null
    }
  } catch (error) {
    console.error('Failed to load team libraries:', error)
    teamLibraries.value = []
    selectedTeamLibraryId.value = null
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    teamLibrariesLoading.value = false
  }
}

const loadTeamLibraryGroups = async (libraryID: string, requestID: number) => {
  teamLibraryGroupsLoading.value = true
  try {
    const groups = await ChatWikiService.GetLibraryGroup(libraryID, 1)
    if (requestID !== teamNormalRequestID) return
    teamLibraryGroups.value = groups || []
    const allGroup = teamLibraryGroups.value.find((g) => g.id === TEAM_ALL_GROUP_ID)
    teamNormalTotal.value = allGroup != null && allGroup.total != null ? Number(allGroup.total) : -1
    selectedTeamGroupId.value = TEAM_ALL_GROUP_ID
    teamNormalPage.value = 'groups'
  } catch (error) {
    if (requestID !== teamNormalRequestID) return
    teamLibraryGroups.value = []
    teamNormalTotal.value = -1
    selectedTeamGroupId.value = TEAM_ALL_GROUP_ID
    teamNormalPage.value = 'groups'
    console.error('Failed to load team library groups:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    if (requestID === teamNormalRequestID) {
      teamLibraryGroupsLoading.value = false
    }
  }
}

const loadTeamLibraryFiles = async (
  libraryID: string,
  requestID: number,
  append = false,
  groupID = ''
) => {
  if (append) {
    if (!teamLibraryFilesHasMore.value || teamLibraryFilesLoading.value || teamLibraryFilesLoadingMore.value) {
      return
    }
    teamLibraryFilesLoadingMore.value = true
  } else {
    teamLibraryFilesPage.value = 1
    teamLibraryFilesHasMore.value = true
    teamLibraryFilesLoading.value = true
  }
  try {
    const page = append ? teamLibraryFilesPage.value + 1 : 1
    const result = await ChatWikiService.GetLibFileList(
      libraryID,
      '',
      page,
      TEAM_PAGE_SIZE,
      '',
      '',
      groupID,
      teamNormalSearchKeyword.value
    )
    if (requestID !== teamNormalRequestID) return
    const list = result?.list ?? []
    const total = Number(result?.total ?? -1)
    teamLibraryFiles.value = append ? [...teamLibraryFiles.value, ...list] : list
    teamLibraryFilesPage.value = page
    if (total >= 0) {
      teamLibraryFilesHasMore.value = page * TEAM_PAGE_SIZE < total
    } else {
      teamLibraryFilesHasMore.value = list.length >= TEAM_PAGE_SIZE
    }
    if (!append && teamLibraryTab.value === 3 && groupID === '') {
      teamWechatTotal.value = total >= 0 ? total : -1
    }
  } catch (error) {
    if (requestID !== teamNormalRequestID) return
    if (!append) {
      teamLibraryFiles.value = []
    }
    teamLibraryFilesHasMore.value = false
    console.error('Failed to load team library files:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    if (requestID === teamNormalRequestID) {
      teamLibraryFilesLoading.value = false
      teamLibraryFilesLoadingMore.value = false
      void nextTick(() => {
        checkAndLoadMoreTeamFiles()
      })
    }
  }
}

const loadTeamUngroupedFiles = async (libraryID: string, requestID: number, append = false) => {
  if (append) {
    if (
      !teamUngroupedFilesHasMore.value ||
      teamUngroupedFilesLoading.value ||
      teamUngroupedFilesLoadingMore.value
    ) {
      return
    }
    teamUngroupedFilesLoadingMore.value = true
  } else {
    teamUngroupedFilesPage.value = 1
    teamUngroupedFilesHasMore.value = true
    teamUngroupedFilesLoading.value = true
  }
  try {
    const page = append ? teamUngroupedFilesPage.value + 1 : 1
    const result = await ChatWikiService.GetLibFileList(
      libraryID,
      '',
      page,
      TEAM_PAGE_SIZE,
      '',
      '',
      '0',
      teamNormalSearchKeyword.value
    )
    if (requestID !== teamNormalRequestID) return
    const list = result?.list ?? []
    const total = Number(result?.total ?? -1)
    teamUngroupedFiles.value = append ? [...teamUngroupedFiles.value, ...list] : list
    teamUngroupedFilesPage.value = page
    if (total >= 0) {
      teamUngroupedFilesHasMore.value = page * TEAM_PAGE_SIZE < total
    } else {
      teamUngroupedFilesHasMore.value = list.length >= TEAM_PAGE_SIZE
    }
  } catch (error) {
    if (requestID !== teamNormalRequestID) return
    if (!append) {
      teamUngroupedFiles.value = []
    }
    teamUngroupedFilesHasMore.value = false
    console.error('Failed to load team ungrouped files:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    if (requestID === teamNormalRequestID) {
      teamUngroupedFilesLoading.value = false
      teamUngroupedFilesLoadingMore.value = false
      void nextTick(() => {
        checkAndLoadMoreTeamFiles()
      })
    }
  }
}

const reloadTeamNormalContent = async () => {
  if (!selectedTeamLibraryId.value) {
    clearTeamNormalContent()
    return
  }
  const requestID = ++teamNormalRequestID
  if (teamLibraryTab.value === 0) {
    await loadTeamLibraryGroups(selectedTeamLibraryId.value, requestID)
    if (requestID !== teamNormalRequestID) return
    await loadTeamUngroupedFiles(selectedTeamLibraryId.value, requestID)
    return
  }
  teamNormalPage.value = 'files'
  await loadTeamLibraryFiles(selectedTeamLibraryId.value, requestID, false, '')
}

const loadTeamQALibraryParagraphs = async (libraryID: string, requestID: number, append = false) => {
  if (append) {
    if (!teamQAParagraphsHasMore.value || teamQAParagraphsLoading.value || teamQAParagraphsLoadingMore.value) {
      return
    }
    teamQAParagraphsLoadingMore.value = true
  } else {
    teamQAParagraphsPage.value = 1
    teamQAParagraphsHasMore.value = true
    teamQAParagraphsLoading.value = true
  }
  try {
    const page = append ? teamQAParagraphsPage.value + 1 : 1
    const result = await ChatWikiService.GetParagraphList(
      libraryID,
      '',
      page,
      TEAM_PAGE_SIZE,
      -1,
      -1,
      -1,
      -1,
      '',
      '',
      teamQASearchKeyword.value
    )
    if (requestID !== teamNormalRequestID) return
    const nextList = result?.list || []
    const total = Number(result?.total ?? -1)
    teamQAParagraphs.value = append ? [...teamQAParagraphs.value, ...nextList] : nextList
    teamQAParagraphsPage.value = page
    teamQAParagraphTotal.value = Number.isFinite(total) ? total : -1
    if (teamQAParagraphTotal.value >= 0) {
      teamQAParagraphsHasMore.value = page * TEAM_PAGE_SIZE < teamQAParagraphTotal.value
    } else {
      teamQAParagraphsHasMore.value = nextList.length >= TEAM_PAGE_SIZE
    }
  } catch (error) {
    if (requestID !== teamNormalRequestID) return
    if (!append) {
      teamQAParagraphs.value = []
    }
    teamQAParagraphsHasMore.value = false
    console.error('Failed to load team QA paragraphs:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    if (requestID === teamNormalRequestID) {
      teamQAParagraphsLoading.value = false
      teamQAParagraphsLoadingMore.value = false
      void nextTick(() => {
        checkAndLoadMoreTeamFiles()
      })
    }
  }
}

const reloadTeamQAContent = async () => {
  if (!selectedTeamLibraryId.value) {
    clearTeamQAContent()
    return
  }
  const requestID = ++teamNormalRequestID
  await loadTeamQALibraryParagraphs(selectedTeamLibraryId.value, requestID)
}

const applyTeamQASearch = () => {
  const keyword = teamQASearchInput.value.trim()
  if (keyword === teamQASearchKeyword.value) return
  teamQASearchKeyword.value = keyword
  void reloadTeamQAContent()
}

const handleTeamGroupSelect = (groupID: string) => {
  if (!selectedTeamLibraryId.value) return
  selectedTeamGroupId.value = groupID
  teamNormalPage.value = 'files'
  const requestID = ++teamNormalRequestID
  void loadTeamLibraryFiles(selectedTeamLibraryId.value, requestID, false, groupID)
}

const handleBackToTeamGroups = () => {
  teamNormalPage.value = 'groups'
  void nextTick(() => {
    checkAndLoadMoreTeamFiles()
  })
}

const checkAndLoadMoreTeamFiles = () => {
  if (!selectedTeamLibraryId.value) return
  const container = teamScrollContainerRef.value
  if (!container) return
  const remain = container.scrollHeight - container.scrollTop - container.clientHeight
  if (remain > TEAM_LOAD_MORE_THRESHOLD) return

  if (teamLibraryTab.value === 2) {
    if (teamQAParagraphsHasMore.value && !teamQAParagraphsLoading.value && !teamQAParagraphsLoadingMore.value) {
      void loadTeamQALibraryParagraphs(selectedTeamLibraryId.value, teamNormalRequestID, true)
    }
    return
  }

  if (teamLibraryTab.value === 0 && teamNormalPage.value === 'groups') {
    if (
      teamUngroupedFilesHasMore.value &&
      !teamUngroupedFilesLoading.value &&
      !teamUngroupedFilesLoadingMore.value
    ) {
      void loadTeamUngroupedFiles(selectedTeamLibraryId.value, teamNormalRequestID, true)
    }
    return
  }

  if (teamLibraryFilesHasMore.value && !teamLibraryFilesLoading.value && !teamLibraryFilesLoadingMore.value) {
    const groupID = teamLibraryTab.value === 0 ? selectedTeamGroupId.value : ''
    void loadTeamLibraryFiles(selectedTeamLibraryId.value, teamNormalRequestID, true, groupID)
  }
}

const handleTeamFilesScroll = () => {
  if (teamScrollRafID != null) return
  teamScrollRafID = window.requestAnimationFrame(() => {
    teamScrollRafID = null
    checkAndLoadMoreTeamFiles()
  })
}

const checkTeamBindingAndLoad = async () => {
  teamBindingChecked.value = false
  try {
    const binding = await getBindingCached()
    // Valid only when binding exists and exp (Unix seconds) not expired
    const exp = binding?.exp != null ? Number(binding.exp) : 0
    teamBound.value = !!binding && exp > Math.floor(Date.now() / 1000)
    if (!teamBound.value) {
      teamLibraries.value = []
      selectedTeamLibraryId.value = null
      clearTeamQAContent()
      return
    }
    await loadTeamLibraries()
  } catch (error) {
    console.error('Failed to check team binding:', error)
    teamBound.value = false
    teamLibraries.value = []
    selectedTeamLibraryId.value = null
    clearTeamQAContent()
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    teamBindingChecked.value = true
  }
}

const handleCreated = async (lib: Library) => {
  // 立即插入列表（减少一次刷新等待），并选中
  libraries.value = [...libraries.value, lib].sort(
    (a, b) => b.sort_order - a.sort_order || b.id - a.id
  )
  selectedLibraryId.value = lib.id
  expandedLibraries.value.add(lib.id)
  await loadFoldersForLibrary(lib.id)
  toast.success(t('knowledge.create.success'))
}

const handleOpenRename = (lib: Library) => {
  actionLibrary.value = lib
  renameOpen.value = true
}

const handleOpenEdit = (lib: Library) => {
  actionLibrary.value = lib
  editOpen.value = true
}

const handleOpenDelete = (lib: Library) => {
  actionLibrary.value = lib
  deleteOpen.value = true
}

const handleLibraryUpdated = (updated: Library) => {
  libraries.value = libraries.value.map((l) => (l.id === updated.id ? updated : l))
  if (selectedLibraryId.value === updated.id) {
    // selectedLibrary is computed from libraries, no-op
  }
}

const confirmDelete = async () => {
  if (!actionLibrary.value) return
  try {
    await LibraryService.DeleteLibrary(actionLibrary.value.id)
    libraries.value = libraries.value.filter((l) => l.id !== actionLibrary.value?.id)
    if (selectedLibraryId.value === actionLibrary.value.id) {
      selectedLibraryId.value = libraries.value[0]?.id ?? null
    }
    toast.success(t('knowledge.delete.success'))
    deleteOpen.value = false
  } catch (error) {
    console.error('Failed to delete library:', error)
    toast.error(getErrorMessage(error) || t('knowledge.delete.failed'))
  }
}

// When thinking mode changes, show toast notification (same as assistant page)
let isInitialMount = true

onMounted(async () => {
  void loadLibraries()
  await loadAgents()
  await loadModels()
  selectDefaultModel(activeAgent.value, null)
  isInitialMount = false
})

onUnmounted(() => {
  if (teamScrollRafID != null) {
    window.cancelAnimationFrame(teamScrollRafID)
    teamScrollRafID = null
  }
})

// When switching to team tab, always re-check binding and load
watch(activeTab, (tab) => {
  if (tab === 'team') {
    void checkTeamBindingAndLoad()
  }
})

// When this module tab becomes active and we're on team tab but unbound, re-check (e.g. after binding in settings)
const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)
watch(isTabActive, (active) => {
  if (active && activeTab.value === 'team' && !teamBound.value) {
    void checkTeamBindingAndLoad()
  }
})

watch(teamLibraryTab, () => {
  if (activeTab.value !== 'team' || !teamBound.value) return
  clearTeamNormalContent()
  clearTeamQAContent()
  void loadTeamLibraries()
})

watch([activeTab, teamLibraryTab, selectedTeamLibraryId], ([tab, libType, libraryID]) => {
  if (tab !== 'team' || !libraryID) return
  if (libType === 2) {
    void reloadTeamQAContent()
    return
  }
  void reloadTeamNormalContent()
})

// When agent changes, re-select default model
watch(activeAgentId, () => {
  selectDefaultModel(activeAgent.value, null)
})

// When models are loaded, select default model
watch(providersWithModels, () => {
  selectDefaultModel(activeAgent.value, null)
})

watch(enableThinking, (newValue) => {
  if (!isInitialMount) {
    toast.default(newValue ? t('assistant.chat.thinkingOn') : t('assistant.chat.thinkingOff'))
  }
})

// Handle send message
const handleSendMessage = () => {
  if (!canSend.value) return

  const messageContent = chatInput.value.trim()
  const imagesToSend = [...pendingImages.value]

  // Build library IDs array from the selected library
  const libraryIds = selectedLibraryId.value ? [selectedLibraryId.value] : []

  // Set pending chat data and open a new assistant tab
  navigationStore.setPendingChatAndOpenAssistant({
    chatInput: messageContent,
    libraryIds,
    selectedModelKey: selectedModelKey.value,
    agentId: activeAgentId.value ?? undefined,
    enableThinking: enableThinking.value,
    chatMode: chatMode.value,
    ...(imagesToSend.length > 0 && {
      pendingImages: imagesToSend.map((img) => ({
        id: img.id,
        mimeType: img.mimeType,
        base64: img.base64,
        dataUrl: img.dataUrl,
        fileName: img.fileName,
        size: img.size,
      })),
    }),
  })

  // Clear input and images after sending
  chatInput.value = ''
  pendingImages.value = []
}

// Handle add images
// 支持图片识别的模型可以通过调用技能去识别图片，所以不再限制模型能力
const handleAddImages = (files: FileList | File[]) => {
  // const modelInfo = selectedModelInfo.value
  // if (modelInfo && !supportsMultimodal(modelInfo.providerId, modelInfo.modelId, modelInfo.capabilities)) {
  //   toast.error(t('assistant.errors.modelNotSupportVision'))
  //   return
  // }

  for (const file of Array.from(files)) {
    if (!file.type.startsWith('image/')) continue
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = reader.result as string
      const base64Match = dataUrl.match(/^data:image\/[^;]+;base64,(.+)$/)
      const base64 = base64Match ? base64Match[1] : ''
      if (!base64) return
      pendingImages.value.push({
        id: crypto.randomUUID(),
        file,
        mimeType: file.type,
        base64,
        dataUrl,
        fileName: file.name,
        size: file.size,
      })
    }
    reader.readAsDataURL(file)
  }
}

// Handle remove image
const handleRemoveImage = (id: string) => {
  pendingImages.value = pendingImages.value.filter((img) => img.id !== id)
}
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <!-- 左侧：个人/团队 tab 与知识库列表，始终展示以支持切换；支持收起/展开 -->
    <aside
      :class="
        cn(
          'flex shrink-0 flex-col border-r border-border bg-background transition-[width] duration-200',
          sidebarCollapsed ? 'w-14' : 'w-sidebar'
        )
      "
    >
      <div class="flex items-center gap-2 border-b border-border px-2 py-2">
        <Button
          v-if="!sidebarCollapsed"
          variant="ghost"
          size="icon"
          class="h-8 w-8 shrink-0"
          :title="t('knowledge.sidebar.collapse')"
          @click="sidebarCollapsed = true"
        >
          <IconSidebarCollapse class="size-4 text-muted-foreground" />
        </Button>
        <Button
          v-else
          variant="ghost"
          size="icon"
          class="h-8 w-8 shrink-0"
          :title="t('knowledge.sidebar.expand')"
          @click="sidebarCollapsed = false"
        >
          <IconSidebarExpand class="size-4 text-muted-foreground" />
        </Button>
        <template v-if="!sidebarCollapsed">
          <div class="inline-flex min-w-0 flex-1 rounded-md bg-muted p-1">
            <button
              type="button"
              :class="
                cn(
                  'rounded px-3 py-1 text-sm transition-colors',
                  activeTab === 'personal'
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                )
              "
              @click="activeTab = 'personal'"
            >
              {{ t('knowledge.tabs.personal') }}
            </button>
            <button
              type="button"
              :class="
                cn(
                  'rounded px-3 py-1 text-sm transition-colors',
                  activeTab === 'team'
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                )
              "
              @click="activeTab = 'team'"
            >
              {{ t('knowledge.tabs.team') }}
            </button>
          </div>
          <div v-if="activeTab === 'personal'" class="flex shrink-0 items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              class="h-8 w-8"
              :title="t('knowledge.create.title')"
              @click="handleCreateClick"
            >
              <Plus class="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              class="h-8 w-8"
              :title="t('knowledge.embeddingSettings.title')"
              @click="handleEmbeddingSettingsClick"
            >
              <Settings class="size-4" />
            </Button>
          </div>
        </template>
      </div>

      <div class="flex-1 overflow-auto px-2 pb-2 pt-2">
        <div v-if="loading" class="px-2 py-6 text-sm text-muted-foreground">
          {{ sidebarCollapsed ? '' : t('knowledge.loading') }}
        </div>

        <div
          v-else-if="activeTab === 'personal' && libraries.length === 0 && !sidebarCollapsed"
          class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
        >
          <div class="text-center text-sm text-muted-foreground">
            {{ t('knowledge.empty.title') }}
          </div>
        </div>

        <div v-else-if="sidebarCollapsed" class="flex flex-col items-center gap-1">
          <button
            v-if="activeTab === 'personal'"
            v-for="lib in libraries"
            :key="`personal-${lib.id}`"
            type="button"
            :class="
              cn(
                'flex size-9 shrink-0 items-center justify-center rounded-lg border transition-colors',
                selectedLibraryId === lib.id
                  ? 'border-primary bg-accent text-accent-foreground ring-1 ring-primary'
                  : 'border-border bg-card text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground'
              )
            "
            :title="lib.name"
            @click="handleCollapsedPersonalLibraryClick(lib.id)"
          >
            <IconKnowledge class="size-4 shrink-0" />
          </button>
          <button
            v-else
            v-for="lib in teamLibraries"
            :key="`team-${lib.id}`"
            type="button"
            :class="
              cn(
                'flex size-9 shrink-0 items-center justify-center rounded-lg border transition-colors',
                selectedTeamLibraryId === lib.id
                  ? 'border-primary bg-accent text-accent-foreground ring-1 ring-primary'
                  : 'border-border bg-card text-muted-foreground hover:border-muted-foreground/30 hover:text-foreground'
              )
            "
            :title="lib.name"
            @click="handleCollapsedTeamLibraryClick(lib.id)"
          >
            <FileStack class="size-4 shrink-0" />
          </button>
        </div>

        <div v-else-if="activeTab === 'team'" class="flex flex-col gap-1">
          <div
            v-if="!teamBindingChecked || teamLibrariesLoading"
            class="px-2 py-6 text-sm text-muted-foreground"
          >
            {{ t('knowledge.loading') }}
          </div>
          <div
            v-else-if="!teamBound"
            class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
          >
            <div class="text-center text-sm text-muted-foreground">
              {{ t('knowledge.team.notBoundTitle') }}
            </div>
          </div>
          <div v-else class="flex flex-col gap-1">
            <!-- Always show the three category tabs when team is bound -->
            <div class="mx-1 mb-1 inline-flex rounded-md bg-muted p-1">
              <button
                type="button"
                class="rounded px-2 py-1 text-xs transition-colors"
                :class="
                  teamLibraryTab === 0
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click="teamLibraryTab = 0"
              >
                {{ t('settings.chatwiki.libraryType.normal').replace('知识库', '') }}
              </button>
              <button
                type="button"
                class="rounded px-2 py-1 text-xs transition-colors"
                :class="
                  teamLibraryTab === 2
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click="teamLibraryTab = 2"
              >
                {{ t('settings.chatwiki.libraryType.qa').replace('知识库', '') }}
              </button>
              <button
                type="button"
                class="rounded px-2 py-1 text-xs transition-colors"
                :class="
                  teamLibraryTab === 3
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                "
                @click="teamLibraryTab = 3"
              >
                {{ t('settings.chatwiki.libraryType.wechat').replace('知识库', '') }}
              </button>
            </div>
            <div
              v-if="teamLibraries.length === 0"
              class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
            >
              {{ t('knowledge.team.empty') }}
            </div>
            <template v-else>
              <div
                v-for="lib in teamLibraries"
                :key="lib.id"
                class="flex h-10 items-center gap-2 rounded-lg px-2 text-left text-sm font-normal transition-colors"
                :class="
                  cn(
                    selectedTeamLibraryId === lib.id
                      ? 'bg-accent text-accent-foreground'
                      : 'text-foreground hover:bg-accent/50'
                  )
                "
                role="button"
                @click="selectedTeamLibraryId = lib.id"
              >
                <span class="min-w-0 flex-1 truncate" :title="lib.name">
                  {{ lib.name }}
                </span>
              </div>
            </template>
          </div>
        </div>

        <div v-else class="flex flex-col gap-1">
          <div v-for="lib in libraries" :key="lib.id" class="flex flex-col gap-0.5">
            <!-- 知识库项 -->
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
                @click.stop="toggleLibraryExpanded(lib.id)"
              >
                <ChevronRight
                  :class="
                    cn(
                      'size-3.5 transition-transform',
                      expandedLibraries.has(lib.id) && 'rotate-90'
                    )
                  "
                />
              </button>
              <div
                role="button"
                tabindex="0"
                :class="
                  cn(
                    'group flex h-10 flex-1 cursor-pointer items-center gap-2 rounded-lg px-2 text-left text-sm font-normal transition-colors',
                    selectedLibraryId === lib.id
                      ? 'bg-accent text-accent-foreground'
                      : 'text-foreground hover:bg-accent/50'
                  )
                "
                @click="handleLibraryClick(lib.id)"
              >
                <span class="min-w-0 flex-1 truncate" :title="lib.name">
                  {{ lib.name }}
                </span>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground opacity-0 transition-opacity hover:bg-background/60 hover:text-foreground group-hover:opacity-100"
                    :title="t('knowledge.item.menu')"
                    @click.stop
                  >
                    <MoreHorizontal class="size-4" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-40">
                    <DropdownMenuItem class="gap-2" @select="handleOpenRename(lib)">
                      <IconRename class="size-4 text-muted-foreground" />
                      {{ t('knowledge.item.rename') }}
                    </DropdownMenuItem>
                    <DropdownMenuItem class="gap-2" @select="handleOpenEdit(lib)">
                      <IconLibSettings class="size-4 text-muted-foreground" />
                      {{ t('knowledge.item.settings') }}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      class="gap-2 text-muted-foreground focus:text-foreground"
                      @select="handleOpenDelete(lib)"
                    >
                      <IconDelete class="size-4" />
                      {{ t('knowledge.item.delete') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
            <!-- Folder tree -->
            <div
              v-if="expandedLibraries.has(lib.id)"
              class="flex flex-col overflow-hidden border-t border-border/50 px-1 pb-1.5 pt-0.5"
            >
              <!-- Uncategorized option: full-width clickable row -->
              <div
                class="flex min-h-8 w-full cursor-pointer items-center gap-1 rounded-lg transition-colors"
                :class="
                  selectedFolderId === -1 && selectedLibraryId === lib.id
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                "
                @click.stop="handleFolderClick(-1, lib.id)"
              >
                <FileStack class="size-4 shrink-0 text-muted-foreground" />
                <span
                  class="min-w-0 flex-1 truncate text-xs"
                  :title="t('knowledge.folder.uncategorized')"
                >
                  {{ t('knowledge.folder.uncategorized') }}
                </span>
              </div>
              <!-- 文件夹列表 -->
              <template v-if="libraryFolders.has(lib.id)">
                <FolderTreeItem
                  v-for="folder in libraryFolders.get(lib.id) || []"
                  :key="folder.id"
                  :folder="folder"
                  :selected-folder-id="selectedFolderId"
                  :selected-library-id="lib.id"
                  :expanded-folders="expandedFolders"
                  @toggle-expanded="toggleFolderExpanded"
                  @folder-click="(folderId) => handleFolderClick(folderId, lib.id)"
                />
              </template>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <!-- 右侧：内容区 -->
    <main class="flex flex-1 flex-col overflow-hidden bg-background">
      <!-- 团队知识库 -->
      <div
        v-if="activeTab === 'team' && !teamBindingChecked"
        class="flex h-full items-center justify-center px-8"
      >
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('knowledge.loading') }}
        </div>
      </div>

      <div
        v-else-if="activeTab === 'team' && !teamBound"
        class="flex h-full items-center justify-center px-8"
      >
        <div class="flex flex-col items-center gap-4">
          <div class="grid size-10 place-items-center rounded-lg bg-muted">
            <IconKnowledge class="size-4 text-muted-foreground" />
          </div>
          <div class="flex flex-col items-center gap-1.5">
            <h3 class="text-base font-medium text-foreground">
              {{ t('knowledge.team.notBoundTitle') }}
            </h3>
            <p class="text-sm text-muted-foreground">
              {{ t('knowledge.team.needsBinding') }}
            </p>
          </div>
          <Button class="mt-1" @click="goToChatwikiBindingSettings">
            {{ t('knowledge.team.goBind') }}
          </Button>
        </div>
      </div>

      <div
        v-else-if="activeTab === 'team' && teamLibrariesLoading"
        class="flex h-full items-center justify-center px-8"
      >
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('knowledge.loading') }}
        </div>
      </div>

      <div
        v-else-if="activeTab === 'team' && !selectedTeamLibrary"
        class="flex h-full items-center justify-center px-8"
      >
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('knowledge.team.empty') }}
        </div>
      </div>

      <div v-else-if="activeTab === 'team' && teamLibraryTab === 0" class="flex min-h-0 flex-1 flex-col">
        <div class="border-b border-border px-4 py-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-base font-medium text-foreground">{{ selectedTeamLibrary?.name ?? '' }}</h3>
                <span v-if="teamNormalTotal >= 0" class="text-xs text-muted-foreground">
                  {{ t('knowledge.team.fileTotal', { count: teamNormalTotal }) }}
                </span>
              </div>
              <p class="mt-1 text-sm text-muted-foreground">
                {{ selectedTeamLibrary?.intro || t('knowledge.team.noIntro') }}
              </p>
            </div>
            <div class="shrink-0">
              <Input
                v-model="teamNormalSearchInput"
                class="h-8 w-48 md:w-56"
                :placeholder="t('knowledge.content.searchPlaceholder')"
                @keydown.enter="applyTeamNormalSearch"
              />
            </div>
          </div>
        </div>
        <div ref="teamScrollContainerRef" class="flex-1 overflow-auto p-4" @scroll.passive="handleTeamFilesScroll">
          <div
            v-if="teamNormalPage === 'groups' && (teamLibraryGroupsLoading || teamUngroupedFilesLoading)"
            class="flex h-full items-center justify-center"
          >
            <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
          </div>
          <div
            v-else-if="teamNormalPage === 'groups' && teamGroupCards.length === 0 && teamUngroupedFiles.length === 0"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('knowledge.team.noFiles') }}
          </div>
          <div
            v-else-if="teamNormalPage === 'groups'"
            class="grid auto-rows-max gap-4"
            style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
          >
            <button
              v-for="group in teamGroupCards"
              :key="`team-group-${group.id}`"
              type="button"
              class="group rounded-xl border border-border bg-card p-3 text-left shadow-sm transition-colors hover:bg-accent/20 dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
              @click="handleTeamGroupSelect(group.id)"
            >
              <div class="grid h-28 w-full place-items-center rounded-lg bg-muted/60 dark:bg-white/5">
                <FolderIcon class="size-10 text-muted-foreground" />
              </div>
              <p class="mt-2 line-clamp-2 min-h-9 text-sm text-foreground" :title="group.name">
                {{ group.name }}
              </p>
              <div class="mt-1 text-xs text-muted-foreground">
                {{ t('knowledge.team.groupFileCount', { count: Math.max(0, Number(group.total || 0)) }) }}
              </div>
            </button>
            <div
              v-for="file in teamUngroupedFiles"
              :key="`root-ungrouped-${file.id}`"
              class="group rounded-xl border border-border bg-card p-3 shadow-sm transition-colors hover:bg-accent/20 dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <div class="relative mb-2">
                <div
                  v-if="canShowTeamFileThumb(file)"
                  class="grid h-28 w-full place-items-center overflow-hidden rounded-lg bg-muted/40 dark:bg-white/5"
                >
                  <img
                    :src="getTeamFileThumbUrl(file)"
                    alt=""
                    class="h-full w-full object-contain"
                    @error="handleTeamFileThumbError(file)"
                  />
                </div>
                <div
                  v-else
                  class="grid h-28 w-full place-items-center rounded-lg bg-muted/60 dark:bg-white/5"
                >
                  <component :is="getTeamFileIcon(file)" class="size-6 text-muted-foreground" />
                </div>
                <span
                  v-if="getTeamFileStatusLabel(file.status)"
                  class="absolute right-1.5 top-1.5 rounded bg-background/90 px-1.5 py-0.5 text-[10px] text-muted-foreground backdrop-blur"
                >
                  {{ getTeamFileStatusLabel(file.status) }}
                </span>
              </div>
              <p class="line-clamp-2 min-h-9 text-sm text-foreground" :title="file.name">
                {{ file.name }}
              </p>
              <div class="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                <span>{{ getTeamFileExtension(file) || '-' }}</span>
                <span>{{ formatTeamFileDate(file.updated_at) }}</span>
              </div>
            </div>
            <div
              v-for="idx in (teamUngroupedFilesLoadingMore ? TEAM_SKELETON_COUNT : 0)"
              :key="`root-ungrouped-skeleton-${idx}`"
              class="rounded-xl border border-border bg-card p-3 dark:border-white/15 dark:ring-1 dark:ring-white/5"
            >
              <div class="h-28 w-full animate-pulse rounded-lg bg-muted/50 dark:bg-white/10" />
              <div class="mt-3 h-3 w-4/5 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
              <div class="mt-2 h-3 w-1/2 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
            </div>
          </div>
          <div v-else class="mb-3 flex items-center gap-2">
            <Button variant="outline" size="sm" @click="handleBackToTeamGroups">
              {{ t('settings.chatwiki.back') }}
            </Button>
            <p class="text-sm text-muted-foreground">
              {{ selectedTeamGroupName || t('knowledge.team.allGroups') }}
            </p>
          </div>
          <div v-if="teamNormalPage === 'files' && teamLibraryFilesLoading" class="flex h-full items-center justify-center">
            <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
          </div>
          <div
            v-else-if="teamNormalPage === 'files' && teamLibraryFiles.length === 0"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('knowledge.team.noFiles') }}
          </div>
          <div
            v-else-if="teamNormalPage === 'files'"
            class="grid auto-rows-max gap-4"
            style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
          >
            <div
              v-for="file in teamLibraryFiles"
              :key="file.id"
              class="group rounded-xl border border-border bg-card p-3 shadow-sm transition-colors hover:bg-accent/20 dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <div class="relative mb-2">
                <div
                  v-if="canShowTeamFileThumb(file)"
                  class="grid h-28 w-full place-items-center overflow-hidden rounded-lg bg-muted/40 dark:bg-white/5"
                >
                  <img
                    :src="getTeamFileThumbUrl(file)"
                    alt=""
                    class="h-full w-full object-contain"
                    @error="handleTeamFileThumbError(file)"
                  />
                </div>
                <div
                  v-else
                  class="grid h-28 w-full place-items-center rounded-lg bg-muted/60 dark:bg-white/5"
                >
                  <component :is="getTeamFileIcon(file)" class="size-6 text-muted-foreground" />
                </div>
                <span
                  v-if="getTeamFileStatusLabel(file.status)"
                  class="absolute right-1.5 top-1.5 rounded bg-background/90 px-1.5 py-0.5 text-[10px] text-muted-foreground backdrop-blur"
                >
                  {{ getTeamFileStatusLabel(file.status) }}
                </span>
              </div>
              <p class="line-clamp-2 min-h-9 text-sm text-foreground" :title="file.name">
                {{ file.name }}
              </p>
              <div class="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                <span>{{ getTeamFileExtension(file) || '-' }}</span>
                <span>{{ formatTeamFileDate(file.updated_at) }}</span>
              </div>
            </div>
            <div
              v-for="idx in (teamLibraryFilesLoadingMore ? TEAM_SKELETON_COUNT : 0)"
              :key="`team-files-skeleton-${idx}`"
              class="rounded-xl border border-border bg-card p-3 dark:border-white/15 dark:ring-1 dark:ring-white/5"
            >
              <div class="h-28 w-full animate-pulse rounded-lg bg-muted/50 dark:bg-white/10" />
              <div class="mt-3 h-3 w-4/5 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
              <div class="mt-2 h-3 w-1/2 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="activeTab === 'team' && teamLibraryTab === 3" class="flex min-h-0 flex-1 flex-col">
        <div class="border-b border-border px-4 py-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-base font-medium text-foreground">{{ selectedTeamLibrary?.name ?? '' }}</h3>
                <span v-if="teamWechatTotal >= 0" class="text-xs text-muted-foreground">
                  {{ t('knowledge.team.fileTotal', { count: teamWechatTotal }) }}
                </span>
              </div>
              <p class="mt-1 text-sm text-muted-foreground">
                {{ selectedTeamLibrary?.intro || t('knowledge.team.noIntro') }}
              </p>
            </div>
            <div class="shrink-0">
              <Input
                v-model="teamNormalSearchInput"
                class="h-8 w-48 md:w-56"
                :placeholder="t('knowledge.content.searchPlaceholder')"
                @keydown.enter="applyTeamNormalSearch"
              />
            </div>
          </div>
        </div>
        <div ref="teamScrollContainerRef" class="flex-1 overflow-auto p-4" @scroll.passive="handleTeamFilesScroll">
          <div v-if="teamLibraryFilesLoading" class="flex h-full items-center justify-center">
            <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
          </div>
          <div
            v-else-if="teamLibraryFiles.length === 0"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('knowledge.team.noFiles') }}
          </div>
          <div
            v-else
            class="grid auto-rows-max gap-4"
            style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
          >
            <div
              v-for="file in teamLibraryFiles"
              :key="file.id"
              class="group rounded-xl border border-border bg-card p-3 shadow-sm transition-colors hover:bg-accent/20 dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <div class="relative mb-2">
                <div
                  v-if="canShowTeamFileThumb(file)"
                  class="grid h-28 w-full place-items-center overflow-hidden rounded-lg bg-muted/40 dark:bg-white/5"
                >
                  <img
                    :src="getTeamFileThumbUrl(file)"
                    alt=""
                    class="h-full w-full object-contain"
                    @error="handleTeamFileThumbError(file)"
                  />
                </div>
                <div
                  v-else
                  class="grid h-28 w-full place-items-center rounded-lg bg-muted/60 dark:bg-white/5"
                >
                  <component :is="getTeamFileIcon(file)" class="size-6 text-muted-foreground" />
                </div>
                <span
                  v-if="getTeamFileStatusLabel(file.status)"
                  class="absolute right-1.5 top-1.5 rounded bg-background/90 px-1.5 py-0.5 text-[10px] text-muted-foreground backdrop-blur"
                >
                  {{ getTeamFileStatusLabel(file.status) }}
                </span>
              </div>
              <p class="line-clamp-2 min-h-9 text-sm text-foreground" :title="file.name">
                {{ file.name }}
              </p>
              <div class="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                <span>{{ getTeamFileExtension(file) || '-' }}</span>
                <span>{{ formatTeamFileDate(file.updated_at) }}</span>
              </div>
            </div>
            <div
              v-for="idx in (teamLibraryFilesLoadingMore ? TEAM_SKELETON_COUNT : 0)"
              :key="`wechat-files-skeleton-${idx}`"
              class="rounded-xl border border-border bg-card p-3 dark:border-white/15 dark:ring-1 dark:ring-white/5"
            >
              <div class="h-28 w-full animate-pulse rounded-lg bg-muted/50 dark:bg-white/10" />
              <div class="mt-3 h-3 w-4/5 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
              <div class="mt-2 h-3 w-1/2 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="activeTab === 'team' && teamLibraryTab === 2" class="flex min-h-0 flex-1 flex-col">
        <div class="border-b border-border px-4 py-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-base font-medium text-foreground">{{ selectedTeamLibrary?.name ?? '' }}</h3>
                <span v-if="teamQAParagraphTotal >= 0" class="text-xs text-muted-foreground">
                  {{ t('knowledge.team.qa.total', { count: teamQAParagraphTotal }) }}
                </span>
              </div>
              <p class="mt-1 text-sm text-muted-foreground">
                {{ selectedTeamLibrary?.intro || t('knowledge.team.noIntro') }}
              </p>
            </div>
            <div class="shrink-0">
              <Input
                v-model="teamQASearchInput"
                class="h-8 w-48 md:w-56"
                :placeholder="t('knowledge.team.qa.searchPlaceholder')"
                @keydown.enter="applyTeamQASearch"
              />
            </div>
          </div>
        </div>
        <div ref="teamScrollContainerRef" class="flex-1 overflow-auto p-4" @scroll.passive="handleTeamFilesScroll">
          <div v-if="teamQAParagraphsLoading" class="flex h-full items-center justify-center">
            <div class="text-sm text-muted-foreground">{{ t('knowledge.loading') }}</div>
          </div>
          <div
            v-else-if="teamQAParagraphs.length === 0"
            class="flex h-full items-center justify-center text-sm text-muted-foreground"
          >
            {{ t('knowledge.team.noParagraphs') }}
          </div>
          <div v-else class="space-y-3">
            <div
              v-for="paragraph in teamQAParagraphs"
              :key="paragraph.id"
              class="rounded-xl border border-border bg-card p-4 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <div class="text-sm font-medium text-foreground">
                {{ t('knowledge.team.qa.question') }} {{ paragraph.question || '-' }}
              </div>
              <div class="mt-2 text-sm text-muted-foreground">
                {{ t('knowledge.team.qa.answer') }} {{ paragraph.answer || '-' }}
              </div>
              <div v-if="paragraph.images.length > 0" class="mt-3 flex flex-wrap gap-2">
                <img
                  v-for="(imgURL, idx) in paragraph.images"
                  :key="`${paragraph.id}-img-${idx}`"
                  :src="imgURL"
                  alt=""
                  class="h-16 w-16 rounded-md border border-border object-cover"
                />
              </div>
            </div>
            <div
              v-for="idx in (teamQAParagraphsLoadingMore ? TEAM_SKELETON_COUNT : 0)"
              :key="`qa-skeleton-${idx}`"
              class="rounded-xl border border-border bg-card p-3 dark:border-white/15 dark:ring-1 dark:ring-white/5"
            >
              <div class="h-3 w-4/5 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
              <div class="mt-3 h-3 w-full animate-pulse rounded bg-muted/50 dark:bg-white/10" />
              <div class="mt-2 h-3 w-2/3 animate-pulse rounded bg-muted/50 dark:bg-white/10" />
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="activeTab === 'team'" class="flex h-full items-center justify-center px-8">
        <div
          class="w-full max-w-3xl rounded-2xl border border-border bg-card p-6 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          <div class="space-y-1 border-b border-border pb-4">
            <h3 class="text-base font-medium text-foreground">{{ selectedTeamLibrary?.name ?? '' }}</h3>
            <p class="text-sm text-muted-foreground">
              {{ selectedTeamLibrary?.intro || t('knowledge.team.noIntro') }}
            </p>
          </div>
          <div class="grid grid-cols-1 gap-3 pt-4 text-sm md:grid-cols-2">
            <div class="rounded-lg bg-muted/30 p-3 dark:bg-white/5">
              <p class="text-xs text-muted-foreground">{{ t('knowledge.team.fields.id') }}</p>
              <p class="mt-1 text-foreground">{{ selectedTeamLibrary?.id ?? '' }}</p>
            </div>
            <div class="rounded-lg bg-muted/30 p-3 dark:bg-white/5">
              <p class="text-xs text-muted-foreground">{{ t('knowledge.team.fields.type') }}</p>
              <p class="mt-1 text-foreground">
                {{ selectedTeamLibrary?.type_name || selectedTeamLibrary?.type || '' }}
              </p>
            </div>
            <div class="rounded-lg bg-muted/30 p-3 dark:bg-white/5">
              <p class="text-xs text-muted-foreground">{{ t('knowledge.team.fields.status') }}</p>
              <p class="mt-1 text-foreground">
                {{
                  selectedTeamLibrary?.chat_claw_switch_status === 1
                    ? t('knowledge.team.status.enabled')
                    : t('knowledge.team.status.disabled')
                }}
              </p>
            </div>
            <div class="rounded-lg bg-muted/30 p-3 dark:bg-white/5">
              <p class="text-xs text-muted-foreground">{{ t('knowledge.team.fields.scope') }}</p>
              <p class="mt-1 text-foreground">{{ t('knowledge.team.scopeChatwiki') }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- 知识库为空 -->
      <div v-else-if="isLibraryEmpty" class="flex h-full items-center justify-center px-8">
        <div class="flex flex-col items-center gap-4">
          <div class="grid size-10 place-items-center rounded-lg bg-muted">
            <IconKnowledge class="size-4 text-muted-foreground" />
          </div>
          <div class="flex flex-col items-center gap-1.5">
            <h3 class="text-base font-medium text-foreground">
              {{ t('knowledge.empty.title') }}
            </h3>
            <p class="text-sm text-muted-foreground">
              {{ t('knowledge.empty.desc') }}
            </p>
          </div>
          <Button class="mt-1" @click="handleCreateClick">
            {{ t('knowledge.empty.createBtn') }}
          </Button>
        </div>
      </div>

      <!-- 未选择知识库 -->
      <div v-else-if="!selectedLibrary" class="flex h-full items-center justify-center px-8">
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('knowledge.selectOne') }}
        </div>
      </div>

      <!-- 知识库内容管理 -->
      <LibraryContentArea
        v-else
        :key="`${selectedLibrary?.id ?? ''}-${selectedFolderId}`"
        :library="selectedLibrary!"
        :selected-folder-id="selectedFolderId"
        @folder-selected="handleFolderSelected"
        @folder-created="handleFolderCreated"
        @folder-updated="handleFolderUpdated"
        @folder-deleted="handleFolderDeleted"
      />

      <!-- Bottom chat input using shared ChatInputArea component (personal tab only; hidden on team tab) -->
      <!-- 分割线在输入框容器上，随输入框高度变化而移动 -->
      <div v-if="!isLibraryEmpty && isPersonalTab" class="bg-background pt-3">
        <ChatInputArea
          mode="knowledge"
          v-model:chat-input="chatInput"
          v-model:chat-mode="chatMode"
          v-model:selected-model-key="selectedModelKey"
          v-model:enable-thinking="enableThinking"
          v-model:active-agent-id="activeAgentId"
          :providers-with-models="providersWithModels"
          :has-models="hasModels"
          :selected-model-info="selectedModelInfo"
          :selected-library-ids="selectedLibraryId ? [selectedLibraryId] : []"
          :libraries="libraries"
          :is-generating="false"
          :can-send="canSend"
          :send-disabled-reason="sendDisabledReason"
          :chat-messages="[]"
          :active-agent="activeAgent"
          :agents="agents"
          :pending-images="pendingImages"
          @send="handleSendMessage"
          @add-images="handleAddImages"
          @remove-image="handleRemoveImage"
        />
      </div>
    </main>

    <CreateLibraryDialog v-model:open="createDialogOpen" @created="handleCreated" />
    <EmbeddingSettingsDialog v-model:open="embeddingSettingsOpen" />
    <RenameLibraryDialog
      v-model:open="renameOpen"
      :library="actionLibrary"
      @updated="handleLibraryUpdated"
    />
    <EditLibraryDialog
      v-model:open="editOpen"
      :library="actionLibrary"
      @updated="handleLibraryUpdated"
    />

    <AlertDialog v-model:open="deleteOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('knowledge.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('knowledge.delete.desc', { name: actionLibrary?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {{ t('knowledge.delete.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-foreground text-background hover:bg-foreground/90"
            @click.prevent="confirmDelete"
          >
            {{ t('knowledge.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
