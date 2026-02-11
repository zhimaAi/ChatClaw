<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import CreateLibraryDialog from './components/CreateLibraryDialog.vue'
import EmbeddingSettingsDialog from './components/EmbeddingSettingsDialog.vue'
import RenameLibraryDialog from './components/RenameLibraryDialog.vue'
import EditLibraryDialog from './components/EditLibraryDialog.vue'
import LibraryContentArea from './components/LibraryContentArea.vue'
import KnowledgeChatInput from './components/KnowledgeChatInput.vue'
import IconRename from '@/assets/icons/library-rename.svg'
import IconLibSettings from '@/assets/icons/library-settings.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
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

import type { Library } from '@bindings/willclaw/internal/services/library'
import { LibraryService } from '@bindings/willclaw/internal/services/library'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'

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
    }
  } catch (error) {
    console.error('Failed to load libraries:', error)
    toast.error(getErrorMessage(error) || t('knowledge.loadFailed'))
  } finally {
    loading.value = false
  }
}

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

const handleCreated = (lib: Library) => {
  // 立即插入列表（减少一次刷新等待），并选中
  libraries.value = [...libraries.value, lib].sort(
    (a, b) => b.sort_order - a.sort_order || b.id - a.id
  )
  selectedLibraryId.value = lib.id
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

onMounted(() => {
  void loadLibraries()
})
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <!-- 左侧：知识库列表（知识库为空时隐藏） -->
    <aside v-if="!isLibraryEmpty" class="flex w-sidebar shrink-0 flex-col border-r border-border">
      <div class="flex items-center justify-between gap-2 px-3 py-3">
        <!-- tabs -->
        <div class="inline-flex rounded-md bg-muted p-1">
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

        <div v-if="activeTab === 'personal'" class="flex items-center gap-1">
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
      </div>

      <div class="flex-1 overflow-auto px-2 pb-2">
        <div v-if="loading" class="px-2 py-6 text-sm text-muted-foreground">
          {{ t('knowledge.loading') }}
        </div>

        <div
          v-else-if="activeTab === 'personal' && libraries.length === 0"
          class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
        >
          <div class="text-center text-sm text-muted-foreground">
            {{ t('knowledge.empty.title') }}
          </div>
        </div>

        <div v-else class="flex flex-col gap-1">
          <button
            v-for="lib in libraries"
            :key="lib.id"
            type="button"
            :class="
              cn(
                'group flex h-10 w-full items-center gap-2 rounded-lg px-2 text-left text-sm font-normal transition-colors',
                selectedLibraryId === lib.id
                  ? 'bg-accent text-accent-foreground'
                  : 'text-foreground hover:bg-accent/50'
              )
            "
            @click="selectedLibraryId = lib.id"
          >
            <span class="min-w-0 flex-1 truncate">{{ lib.name }}</span>
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
          </button>
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
      <LibraryContentArea v-else :key="selectedLibrary.id" :library="selectedLibrary" />

      <!-- Bottom chat input shortcut (hidden when no libraries exist) -->
      <KnowledgeChatInput v-if="!isLibraryEmpty" :selected-library-id="selectedLibraryId" :tab-id="tabId" />
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
