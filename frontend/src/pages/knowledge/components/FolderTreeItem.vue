<script setup lang="ts">
import { ChevronRight, Folder as FolderIcon } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  folder: Folder
  level?: number // 嵌套层级，用于缩进
  selectedFolderId: number | null
  selectedLibraryId: number | null
  expandedFolders: Set<number>
}>()

const emit = defineEmits<{
  toggleExpanded: [folderId: number]
  folderClick: [folderId: number]
}>()

const handleToggleExpanded = (folderId: number) => {
  emit('toggleExpanded', folderId)
}

const handleFolderClick = (folderId: number) => {
  emit('folderClick', folderId)
}
</script>

<template>
  <div class="flex flex-col gap-0.5">
    <!-- 当前文件夹 -->
    <div class="flex items-center gap-1">
      <button
        v-if="folder.children && folder.children.length > 0"
        type="button"
        class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
        @click.stop="handleToggleExpanded(folder.id)"
      >
        <ChevronRight
          :class="cn(
            'size-3.5 transition-transform',
            props.expandedFolders.has(folder.id) && 'rotate-90'
          )"
        />
      </button>
      <div v-else class="size-6 shrink-0" />
      <button
        type="button"
        :class="
          cn(
            'flex h-8 flex-1 items-center gap-2 rounded-lg px-2 text-left text-xs transition-colors',
            props.selectedFolderId === folder.id && props.selectedLibraryId === folder.library_id
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
          )
        "
        @click.stop="handleFolderClick(folder.id)"
      >
        <FolderIcon class="size-4 shrink-0 text-muted-foreground" />
        <span class="min-w-0 flex-1 truncate" :title="folder.name">
          {{ folder.name }}
        </span>
      </button>
    </div>
    <!-- 子文件夹（递归渲染） -->
    <div
      v-if="props.expandedFolders.has(folder.id) && folder.children && folder.children.length > 0"
      class="ml-5 flex flex-col gap-0.5"
    >
      <FolderTreeItem
        v-for="child in folder.children"
        :key="child.id"
        :folder="child"
        :level="(props.level || 0) + 1"
        :selected-folder-id="props.selectedFolderId"
        :selected-library-id="props.selectedLibraryId"
        :expanded-folders="props.expandedFolders"
        @toggle-expanded="handleToggleExpanded"
        @folder-click="handleFolderClick"
      />
    </div>
  </div>
</template>
