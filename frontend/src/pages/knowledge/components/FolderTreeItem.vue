<script setup lang="ts">
import { computed } from 'vue'
import { Folder as FolderIcon, FolderClosed, FolderOpen } from 'lucide-vue-next'
import IconDown from '@/assets/icons/down-icon.svg'
import IconRight from '@/assets/icons/right-icon.svg'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  folder: Folder
  level?: number
  selectedFolderId: number | null
  selectedLibraryId: number | null
  expandedFolders: Set<number>
  rootLibraryId: number | null
}>()

const emit = defineEmits<{
  toggleExpanded: [folderId: number]
  folderClick: [folderId: number]
}>()

const hasChildren = computed(() => !!props.folder.children && props.folder.children.length > 0)
const isExpanded = computed(() => props.expandedFolders.has(props.folder.id))

const isSelected = computed(() => {
  return (
    props.selectedFolderId === props.folder.id && props.rootLibraryId === props.selectedLibraryId
  )
})

const handleToggleExpanded = (folderId: number) => {
  emit('toggleExpanded', folderId)
}

const handleFolderClick = (folderId: number, event?: Event) => {
  // 阻止 DOM 事件冒泡
  if (event) {
    event.stopPropagation()
  }
  emit('folderClick', folderId)
  // 点击时同时切换展开/收起状态
  if (hasChildren.value) {
    emit('toggleExpanded', folderId)
  }
}
</script>

<template>
  <div class="flex w-full flex-col">
    <!-- Current folder row: full-width clickable -->
    <div
      class="group flex min-h-10 w-full cursor-pointer items-center gap-1 rounded-lg transition-colors"
      :style="{ paddingLeft: `${(props.level || 0) * 20}px` }"
      :class="
        isSelected
          ? 'bg-accent text-accent-foreground'
          : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
      "
      @click="handleFolderClick(folder.id, $event)"
    >
      <!-- Expand/collapse arrow (only when has children) -->
      <span
        v-if="hasChildren"
        class="flex size-4 shrink-0 items-center justify-center text-muted-foreground"
        aria-hidden
      >
        <IconDown v-if="isExpanded" class="size-4" />
        <IconRight v-else class="size-4" />
      </span>
      <span v-else class="flex size-4 shrink-0" aria-hidden />
      <!-- Folder state icon (display only) -->
      <span class="flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground">
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
      class="flex w-full flex-col"
    >
      <FolderTreeItem
        v-for="child in folder.children"
        :key="child.id"
        :folder="child"
        :level="(props.level || 0) + 1"
        :selected-folder-id="props.selectedFolderId"
        :selected-library-id="props.selectedLibraryId"
        :expanded-folders="props.expandedFolders"
        :root-library-id="props.rootLibraryId"
        @toggle-expanded="handleToggleExpanded"
        @folder-click="handleFolderClick"
      />
    </div>
  </div>
</template>
