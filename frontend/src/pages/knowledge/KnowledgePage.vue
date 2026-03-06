<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, MoreHorizontal, Settings } from 'lucide-vue-next'
import IconKnowledge from '@/assets/icons/knowledge.svg'

/**
 * Props - 每个标签页实例都有自己独立的 tabId
 * 通过 v-show 控制显示/隐藏，组件实例不会被销毁，状态自然保留
 */
defineProps<{
  tabId: string
}>()
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { getErrorMessage } from '@/composables/useErrorMessage'
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
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import { Book, BookOpen, FileStack } from 'lucide-vue-next'
import { useAgents } from '@/pages/assistant/composables/useAgents'
import { useModelSelection } from '@/pages/assistant/composables/useModelSelection'
import { useNavigationStore } from '@/stores'
import { toast } from '@/components/ui/toast'

type LibraryTab = 'personal' | 'team'

const { t } = useI18n()

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

const navigationStore = useNavigationStore()

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

// Whether the library list is empty (loaded & no personal libraries)
const isLibraryEmpty = computed(
  () => !loading.value && activeTab.value === 'personal' && libraries.value.length === 0
)

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

  // 如果点击的是有下级文件夹的文件夹，自动展开下级
  if (folderId > 0) {
    const folders = libraryFolders.value.get(libraryId) || []
    const findFolder = (folders: Folder[], id: number): Folder | null => {
      for (const folder of folders) {
        if (folder.id === id) return folder
        if (folder.children) {
          const found = findFolder(folder.children, id)
          if (found) return found
        }
      }
      return null
    }
    const folder = findFolder(folders, folderId)
    if (folder && folder.children && folder.children.length > 0) {
      expandedFolders.value.add(folderId)
    }
  }
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

onMounted(async () => {
  void loadLibraries()
  await loadAgents()
  await loadModels()
  selectDefaultModel(activeAgent.value, null)
})

// When agent changes, re-select default model
watch(activeAgentId, () => {
  selectDefaultModel(activeAgent.value, null)
})

// When models are loaded, select default model
watch(providersWithModels, () => {
  selectDefaultModel(activeAgent.value, null)
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
const handleAddImages = (files: FileList | File[]) => {
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
    <!-- 左侧：知识库列表（知识库为空时隐藏），支持收起/展开 -->
    <aside
      v-if="!isLibraryEmpty"
      :class="
        cn(
          'flex shrink-0 flex-col border-r border-border bg-background transition-[width] duration-200',
          sidebarCollapsed ? 'w-14' : 'w-sidebar'
        )
      "
    >
      <!-- Header: tabs + actions when expanded; collapse toggle always -->
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
              disabled
              :class="
                cn('rounded px-3 py-1 text-sm transition-colors', 'cursor-not-allowed opacity-50')
              "
              :title="t('knowledge.tabs.teamDisabledTip')"
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

        <!-- Collapsed: only library icons -->
        <div v-else-if="sidebarCollapsed" class="flex flex-col items-center gap-1">
          <button
            v-for="lib in libraries"
            :key="lib.id"
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
            @click="
              selectedLibraryId = lib.id;
              selectedFolderId = null;
              if (!expandedLibraries.has(lib.id)) {
                expandedLibraries.add(lib.id);
                loadFoldersForLibrary(lib.id);
              }
              sidebarCollapsed = false;
            "
          >
            <IconKnowledge class="size-4 shrink-0" />
          </button>
        </div>

        <!-- Expanded: full tree with borders and icons -->
        <div v-else class="flex flex-col gap-2 overflow-hidden">
          <div
            v-for="lib in libraries"
            :key="lib.id"
            class="overflow-hidden rounded-lg border border-border bg-card transition-colors hover:border-muted-foreground/50 dark:hover:border-white/30"
          >
            <!-- Library item: full-width clickable row with expand + menu -->
            <div
              class="group flex min-h-9 w-full min-w-0 cursor-pointer items-center gap-1 overflow-hidden rounded-md p-1 transition-colors"
              :class="
                selectedLibraryId === lib.id && selectedFolderId === null
                  ? 'bg-accent text-accent-foreground'
                  : 'text-foreground hover:bg-accent/50'
              "
              @click="handleLibraryClick(lib.id)"
            >
              <button
                type="button"
                class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
                @click.stop="toggleLibraryExpanded(lib.id)"
              >
                <component
                  :is="expandedLibraries.has(lib.id) ? BookOpen : Book"
                  class="size-4 shrink-0"
                />
              </button>
              <span class="min-w-0 flex-1 truncate text-sm font-normal" :title="lib.name">
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
      <!-- 团队知识库暂未开放 -->
      <div v-if="activeTab !== 'personal'" class="flex h-full items-center justify-center px-8">
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('knowledge.teamNotReady') }}
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
        :key="`${selectedLibrary.id}-${selectedFolderId}`"
        :library="selectedLibrary"
        :selected-folder-id="selectedFolderId"
        @folder-selected="handleFolderSelected"
        @folder-created="handleFolderCreated"
        @folder-updated="handleFolderUpdated"
        @folder-deleted="handleFolderDeleted"
      />

      <!-- Bottom chat input using shared ChatInputArea component -->
      <div v-if="!isLibraryEmpty" class="border-t border-border">
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
