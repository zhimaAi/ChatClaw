<script setup lang="ts">
import { computed } from 'vue'
import { Folder as FolderIcon, FolderClosed, FolderOpen } from 'lucide-vue-next'
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

const hasChildren = computed(() => !!props.folder.children && props.folder.children.length > 0)
const isExpanded = computed(() => props.expandedFolders.has(props.folder.id))

const handleToggleExpanded = (folderId: number) => {
  emit('toggleExpanded', folderId)
}

const handleFolderClick = (folderId: number) => {
  emit('folderClick', folderId)
}
</script>

<template>
  <div class="flex flex-col">
    <!-- Current folder row: full-width clickable, indentation is visual only -->
    <div
      class="group flex min-h-8 w-full cursor-pointer items-center gap-1 rounded-lg transition-colors"
      :style="{ paddingLeft: `${(props.level || 0) * 20}px` }"
      :class="
        props.selectedFolderId === folder.id && props.selectedLibraryId === folder.library_id
          ? 'bg-accent text-accent-foreground'
          : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
      "
      @click="handleFolderClick(folder.id)"
    >
      <!-- Folder state icon (clickable for expand/collapse) -->
      <span
        class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground"
        :class="hasChildren && 'cursor-pointer hover:text-foreground'"
        @click.stop="hasChildren && handleToggleExpanded(folder.id)"
      >
        <component
          :is="hasChildren ? (isExpanded ? FolderOpen : FolderClosed) : FolderIcon"
          class="size-4 shrink-0"
        />
      </span>
      <span class="min-w-0 flex-1 truncate text-xs" :title="folder.name">
        {{ folder.name }}
      </span>
    </div>
    <!-- Child folders (recursive) -->
    <div
      v-if="props.expandedFolders.has(folder.id) && folder.children && folder.children.length > 0"
      class="flex flex-col"
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
