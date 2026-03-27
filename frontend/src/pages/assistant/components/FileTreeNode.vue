<script setup lang="ts">
import { computed } from 'vue'
import { ChevronRight, ChevronDown, File, Folder } from 'lucide-vue-next'
import type { FileEntry } from '@bindings/chatclaw/internal/services/agents'

const props = defineProps<{
  entry: FileEntry
  depth?: number
  expandedDirs: Set<string>
}>()

const emit = defineEmits<{
  toggle: [path: string]
}>()

const depth = computed(() => props.depth ?? 0)
const isExpanded = computed(() => props.expandedDirs.has(props.entry.path))
const paddingLeft = computed(() => `${depth.value * 16 + 4}px`)
const filePaddingLeft = computed(() => `${depth.value * 16 + 4 + 14}px`)
</script>

<template>
  <div>
    <!-- Directory -->
    <template v-if="entry.is_dir">
      <button
        class="flex w-full items-center gap-1.5 rounded px-1 py-1 text-left text-xs text-foreground/80 transition-colors hover:bg-muted/60"
        :style="{ paddingLeft }"
        @click="emit('toggle', entry.path)"
      >
        <ChevronDown v-if="isExpanded" class="size-3 shrink-0 text-muted-foreground" />
        <ChevronRight v-else class="size-3 shrink-0 text-muted-foreground" />
        <Folder class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="min-w-0 truncate">{{ entry.name }}</span>
      </button>
      <template v-if="isExpanded && entry.children">
        <FileTreeNode
          v-for="child in entry.children"
          :key="child.path"
          :entry="child"
          :depth="depth + 1"
          :expanded-dirs="expandedDirs"
          @toggle="(path) => emit('toggle', path)"
        />
      </template>
    </template>

    <!-- File -->
    <div
      v-else
      class="flex items-center gap-1.5 rounded px-1 py-1 text-xs text-foreground/70"
      :style="{ paddingLeft: filePaddingLeft }"
    >
      <File class="size-3.5 shrink-0 text-muted-foreground/70" />
      <span class="min-w-0 truncate">{{ entry.name }}</span>
    </div>
  </div>
</template>
