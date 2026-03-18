<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowLeft, ChevronRight, LoaderCircle, Folder as FolderIcon } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  LibraryService,
  MoveDocumentToFolderInput,
} from '@bindings/chatclaw/internal/services/library'
import type { Folder } from '@bindings/chatclaw/internal/services/library'
import type { Document } from './DocumentCard.vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  open: boolean
  document: Document | null
  folders: Folder[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  moved: []
}>()

const { t } = useI18n()
const moving = ref(false)

type Location = { kind: 'root' } | { kind: 'uncategorized' } | { kind: 'folder'; id: number }

const location = ref<Location>({ kind: 'root' })

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
  if (!canGoBack.value || moving.value) return
  if (location.value.kind === 'uncategorized') {
    location.value = { kind: 'root' }
    return
  }
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
    label: t('knowledge.content.moveToFolder.root'),
    to: { kind: 'root' },
  }

  if (location.value.kind === 'root') return [root]

  if (location.value.kind === 'uncategorized') {
    return [
      root,
      {
        key: 'uncategorized',
        label: t('knowledge.content.moveToFolder.uncategorized'),
        to: { kind: 'uncategorized' },
      },
    ]
  }

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

const canMoveHere = computed(() => location.value.kind !== 'root' && !moving.value)

const close = () => emit('update:open', false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    location.value = { kind: 'root' }
    moving.value = false
  }
)

const handleMove = async () => {
  if (!props.document || moving.value) return
  if (location.value.kind === 'root') return
  moving.value = true
  try {
    let folderID: number | null = null
    if (location.value.kind === 'folder') {
      folderID = location.value.id
    } else if (location.value.kind === 'uncategorized') {
      folderID = null
    }

    await LibraryService.MoveDocumentToFolder(
      new MoveDocumentToFolderInput({
        document_id: props.document.id,
        folder_id: folderID,
      })
    )
    emit('moved')
    toast.success(t('knowledge.content.moveToFolder.success'))
    close()
  } catch (error) {
    console.error('Failed to move document:', error)
    toast.error(getErrorMessage(error) || t('knowledge.content.moveToFolder.failed'))
  } finally {
    moving.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="close">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.content.moveToFolder.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('knowledge.content.moveToFolder.selectFolder') }}
          </label>

          <div class="rounded-md border border-border">
            <!-- Explorer-like header: back + breadcrumbs -->
            <div class="flex items-center gap-2 border-b border-border px-2 py-2">
              <button
                type="button"
                :disabled="!canGoBack || moving"
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
                    :disabled="moving"
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
                <!-- Root-only: allow entering Uncategorized -->
                <button
                  v-if="location.kind === 'root'"
                  type="button"
                  :disabled="moving"
                  class="flex h-10 items-center gap-3 px-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
                  @click="location = { kind: 'uncategorized' }"
                >
                  <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
                  <span
                    class="min-w-0 flex-1 truncate"
                    :title="t('knowledge.content.moveToFolder.uncategorized')"
                  >
                    {{ t('knowledge.content.moveToFolder.uncategorized') }}
                  </span>
                  <ChevronRight class="size-4 shrink-0 text-muted-foreground/70" />
                </button>

                <template v-for="folder in listFolders" :key="folder.id">
                  <button
                    type="button"
                    :disabled="moving"
                    class="flex h-10 items-center gap-3 px-3 text-left text-sm text-foreground transition-colors hover:bg-accent/50 disabled:cursor-not-allowed disabled:opacity-50"
                    @click="location = { kind: 'folder', id: folder.id }"
                  >
                    <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
                    <span class="min-w-0 flex-1 truncate" :title="folder.name">
                      {{ folder.name }}
                    </span>
                    <ChevronRight class="size-4 shrink-0 text-muted-foreground/70" />
                  </button>
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
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="moving" @click="close">
          {{ t('knowledge.create.cancel') }}
        </Button>
        <Button class="gap-2" :disabled="!canMoveHere" @click="handleMove">
          <LoaderCircle v-if="moving" class="size-4 shrink-0 animate-spin" />
          {{ t('knowledge.content.moveToFolder.moveToHere') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
