<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ArrowLeft,
  ChevronRight,
  LoaderCircle,
  Folder as FolderIcon,
  CheckCircle2,
  ChevronDown,
} from 'lucide-vue-next'
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
import { LibraryService, CreateFolderInput } from '@bindings/chatclaw/internal/services/library'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  open: boolean
  libraryId: number
  folders: Folder[]
  defaultParentId?: number | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: [folder: Folder]
}>()

const { t } = useI18n()
const name = ref('')
const saving = ref(false)
const NAME_MAX_LEN = 50

type Location = { kind: 'root' } | { kind: 'folder'; id: number }

const location = ref<Location>({ kind: 'root' })
const selectedParent = ref<Location>({ kind: 'root' })
const showFolderBrowser = ref(false)

type FolderIndexItem = {
  folder: Folder
  parentId: number | null
}

// Build a fast index for breadcrumb navigation.
const folderIndex = computed(() => {
  const index = new Map<number, FolderIndexItem>()
  const walk = (items: Folder[], parentId: number | null) => {
    for (const f of items) {
      index.set(f.id, { folder: f, parentId })
      if (f.children && f.children.length > 0) {
        walk(f.children, f.id)
      }
    }
  }
  walk(props.folders, null)
  return index
})

const currentFolder = computed(() => {
  if (location.value.kind !== 'folder') return null
  return folderIndex.value.get(location.value.id)?.folder ?? null
})

const canGoBack = computed(() => location.value.kind !== 'root')

const goBack = () => {
  if (!canGoBack.value || saving.value) return
  if (location.value.kind === 'folder') {
    const parentId = folderIndex.value.get(location.value.id)?.parentId ?? null
    location.value = parentId ? { kind: 'folder', id: parentId } : { kind: 'root' }
  }
}

type Crumb = {
  key: string
  label: string
  title?: string
  to: Location
}

const breadcrumbs = computed<Crumb[]>(() => {
  const root: Crumb = {
    key: 'root',
    label: t('knowledge.folder.rootFolder'),
    to: { kind: 'root' },
  }

  if (location.value.kind === 'root') return [root]

  // folder path
  const pathIds: number[] = []
  let cursor: number | null = location.value.id
  while (cursor) {
    pathIds.push(cursor)
    cursor = folderIndex.value.get(cursor)?.parentId ?? null
  }
  pathIds.reverse()

  const pathCrumbs: Crumb[] = pathIds.map((id) => {
    const name = folderIndex.value.get(id)?.folder?.name ?? String(id)
    return {
      key: `folder-${id}`,
      label: name,
      title: name,
      to: { kind: 'folder', id },
    }
  })

  return [root, ...pathCrumbs]
})

const listFolders = computed<Folder[]>(() => {
  if (location.value.kind === 'folder') {
    return currentFolder.value?.children ?? []
  }
  if (location.value.kind === 'root') {
    return props.folders
  }
  return []
})

const close = () => emit('update:open', false)

watch(
  () => props.open,
  (open) => {
    if (!open) {
      showFolderBrowser.value = false
      return
    }
    name.value = ''
    // 如果传入了默认父文件夹ID，则使用它；否则使用根目录
    if (
      props.defaultParentId !== null &&
      props.defaultParentId !== undefined &&
      props.defaultParentId > 0
    ) {
      const defaultLocation = { kind: 'folder' as const, id: props.defaultParentId }
      location.value = defaultLocation
      selectedParent.value = defaultLocation
    } else {
      location.value = { kind: 'root' }
      selectedParent.value = { kind: 'root' }
    }
    saving.value = false
    showFolderBrowser.value = false
  }
)

const isValid = computed(() => {
  const n = name.value.trim()
  return n.length > 0 && n.length <= NAME_MAX_LEN
})

const handleCreate = async () => {
  if (!isValid.value || saving.value) return
  saving.value = true
  try {
    // Convert selectedParent to parent_id: null for root, folder id for folder
    const parentIDValue = selectedParent.value.kind === 'root' ? null : selectedParent.value.id
    const created = await LibraryService.CreateFolder(
      new CreateFolderInput({
        library_id: props.libraryId,
        parent_id: parentIDValue,
        name: name.value.trim(),
      })
    )
    if (!created) throw new Error(t('knowledge.folder.createFailed'))
    emit('created', created)
    toast.success(t('knowledge.folder.createSuccess'))
    close()
  } catch (error) {
    console.error('Failed to create folder:', error)
    toast.error(getErrorMessage(error) || t('knowledge.folder.createFailed'))
  } finally {
    saving.value = false
  }
}

// Check if a location is selected as parent
const isSelectedParent = (loc: Location): boolean => {
  if (selectedParent.value.kind === 'root' && loc.kind === 'root') return true
  if (selectedParent.value.kind === 'folder' && loc.kind === 'folder') {
    return selectedParent.value.id === loc.id
  }
  return false
}

// Get display name for selected parent
const selectedParentName = computed(() => {
  if (selectedParent.value.kind === 'root') {
    return t('knowledge.folder.rootFolder')
  }
  return (
    folderIndex.value.get(selectedParent.value.id)?.folder?.name ?? String(selectedParent.value.id)
  )
})

// Handle double click to enter folder
let clickTimer: ReturnType<typeof setTimeout> | null = null
const handleFolderClick = (folder: Folder) => {
  if (saving.value) return

  // Single click: select as parent
  if (clickTimer === null) {
    clickTimer = setTimeout(() => {
      selectedParent.value = { kind: 'folder', id: folder.id }
      showFolderBrowser.value = false
      clickTimer = null
    }, 300) // 300ms delay to detect double click
  } else {
    // Double click: enter folder
    clearTimeout(clickTimer)
    clickTimer = null
    if (folder.children && folder.children.length > 0) {
      location.value = { kind: 'folder', id: folder.id }
    }
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.folder.createTitle') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.folder.parentFolder')"
            :help="t('knowledge.folder.parentFolderHelp')"
          />
          <!-- Current parent display button -->
          <button
            v-if="!showFolderBrowser"
            type="button"
            :disabled="saving"
            class="flex h-10 w-full items-center justify-between rounded-md border border-border bg-background px-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
            @click="location = selectedParent; showFolderBrowser = true"
          >
            <div class="flex min-w-0 flex-1 items-center gap-2">
              <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate" :title="selectedParentName">
                {{ selectedParentName }}
              </span>
            </div>
            <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
          </button>

          <!-- Folder browser -->
          <div v-if="showFolderBrowser" class="rounded-md border border-border">
            <!-- Explorer-like header: back + breadcrumbs -->
            <div class="flex items-center gap-2 border-b border-border px-2 py-2">
              <button
                type="button"
                :disabled="!canGoBack || saving"
                class="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                :title="t('knowledge.content.moveToFolder.back')"
                @click="goBack"
              >
                <ArrowLeft class="size-4" />
              </button>

              <nav class="min-w-0 flex flex-1 items-center gap-1 text-xs text-muted-foreground">
                <template v-for="(c, idx) in breadcrumbs" :key="c.key">
                  <span v-if="idx !== 0" class="px-0.5 text-muted-foreground/60">/</span>
                  <button
                    type="button"
                    class="min-w-0 max-w-[160px] truncate rounded px-1 py-0.5 text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                    :class="idx === breadcrumbs.length - 1 && 'text-foreground'"
                    :title="c.title || c.label"
                    :disabled="saving"
                    @click="location = c.to"
                  >
                    {{ c.label }}
                  </button>
                </template>
              </nav>
            </div>

            <!-- Explorer-like list -->
            <div class="max-h-[300px] overflow-y-auto">
              <div class="flex flex-col">
                <!-- Root option: show when at root, allow selecting root as parent -->
                <button
                  v-if="location.kind === 'root'"
                  type="button"
                  :disabled="saving"
                  class="flex h-10 items-center gap-3 px-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
                  :class="isSelectedParent({ kind: 'root' }) && 'bg-accent/30'"
                  @click="selectedParent = { kind: 'root' }; showFolderBrowser = false"
                >
                  <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
                  <span class="min-w-0 flex-1 truncate" :title="t('knowledge.folder.rootFolder')">
                    {{ t('knowledge.folder.rootFolder') }}
                  </span>
                  <CheckCircle2
                    v-if="isSelectedParent({ kind: 'root' })"
                    class="size-4 shrink-0 text-primary"
                  />
                </button>

                <!-- Current folder option: show when inside a folder, allow selecting current folder as parent -->
                <button
                  v-if="location.kind === 'folder' && currentFolder"
                  type="button"
                  :disabled="saving"
                  class="flex h-10 items-center gap-3 px-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50 border-b border-border"
                  :class="isSelectedParent({ kind: 'folder', id: location.id }) && 'bg-accent/30'"
                  @click="
                    selectedParent = { kind: 'folder', id: location.id }; showFolderBrowser = false
                  "
                >
                  <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
                  <span class="min-w-0 flex-1 truncate" :title="currentFolder.name">
                    {{ currentFolder.name }}
                  </span>
                  <CheckCircle2
                    v-if="isSelectedParent({ kind: 'folder', id: location.id })"
                    class="size-4 shrink-0 text-primary"
                  />
                </button>

                <template v-for="folder in listFolders" :key="folder.id">
                  <div class="group flex items-center">
                    <button
                      type="button"
                      :disabled="saving"
                      class="flex h-10 flex-1 items-center gap-3 pl-6 pr-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
                      :class="isSelectedParent({ kind: 'folder', id: folder.id }) && 'bg-accent/30'"
                      @click="handleFolderClick(folder)"
                    >
                      <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
                      <span class="min-w-0 flex-1 truncate" :title="folder.name">
                        {{ folder.name }}
                      </span>
                      <CheckCircle2
                        v-if="isSelectedParent({ kind: 'folder', id: folder.id })"
                        class="size-4 shrink-0 text-primary"
                      />
                    </button>
                    <button
                      v-if="folder.children && folder.children.length > 0"
                      type="button"
                      :disabled="saving"
                      class="flex h-10 min-w-12 items-center justify-center px-4 text-muted-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
                      @click.stop="location = { kind: 'folder', id: folder.id }"
                    >
                      <ChevronRight class="size-4 shrink-0" />
                    </button>
                  </div>
                </template>

                <div
                  v-if="location.kind !== 'root' && listFolders.length === 0"
                  class="px-3 py-3 text-xs text-muted-foreground"
                >
                  {{ t('knowledge.content.moveToFolder.empty') }}
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="flex flex-col gap-1.5">
          <FieldLabel
            :label="t('knowledge.create.name')"
            :help="t('knowledge.folder.nameHelp')"
            required
          />
          <Input
            v-model="name"
            :placeholder="t('knowledge.folder.createPlaceholder')"
            :maxlength="NAME_MAX_LEN"
            :disabled="saving"
            @keyup.enter="handleCreate"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="saving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button class="gap-2" :disabled="!isValid || saving" @click="handleCreate">
          <LoaderCircle v-if="saving" class="size-4 shrink-0 animate-spin" />
          {{ t('knowledge.folder.create') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
