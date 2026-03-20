<script setup lang="ts">
import { MoreHorizontal } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import folderIcon from '@/assets/images/folder-icon.png'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import IconRename from '@/assets/icons/library-rename.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import { FolderPlus } from 'lucide-vue-next'
import type { Folder as FolderType } from '@bindings/chatclaw/internal/services/library'

const { t } = useI18n()

const props = defineProps<{
  folder: FolderType
  // Total items inside this folder: sub-folders + documents
  documentCount?: number
  // Formatted latest updated time (e.g. 2026/03/02)
  latestUpdatedAt?: string
}>()

const emit = defineEmits<{
  (e: 'click', folder: FolderType): void
  (e: 'rename', folder: FolderType): void
  (e: 'delete', folder: FolderType): void
  (e: 'move', folder: FolderType): void
}>()

const handleCardClick = () => {
  emit('click', props.folder)
}
</script>

<template>
  <div
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col rounded-xl border border-[#f0f0f0] bg-white shadow-sm transition-[box-shadow,border-color,background-color] duration-200 hover:border-black/[0.08] hover:bg-[#f5f5f5] hover:shadow-[0_2px_12px_rgba(0,0,0,0.07)] dark:border-white/15 dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/5 dark:hover:border-white/25 dark:hover:bg-muted dark:hover:shadow-none dark:hover:ring-white/12"
    @click="handleCardClick"
  >
    <!-- Folder icon area: 6px radius, muted bg per design -->
    <div
      class="relative mx-[7px] mt-[7px] flex h-[86px] w-[150px] items-center justify-center overflow-hidden rounded-md dark:border-border dark:bg-muted"
    >
      <img
        :src="folderIcon"
        alt=""
        class="h-auto w-[120px] max-w-full object-contain select-none"
        draggable="false"
      />
    </div>

    <!-- Hover menu -->
    <DropdownMenu>
      <DropdownMenuTrigger
        class="absolute right-[9px] top-[9px] z-10 flex size-6 items-center justify-center rounded-md bg-black/45 p-1 text-white opacity-0 shadow-none transition-[opacity,background-color] hover:bg-black/55 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-white group-hover:opacity-100 dark:bg-white/20 dark:text-white dark:hover:bg-white/30 dark:focus-visible:ring-offset-card"
        @click.stop
      >
        <MoreHorizontal class="size-4 shrink-0" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" class="w-auto min-w-max">
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('rename', folder)">
          <IconRename class="size-4 text-muted-foreground" />
          {{ t('knowledge.folder.rename') }}
        </DropdownMenuItem>
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('move', folder)">
          <FolderPlus class="size-4 text-muted-foreground" />
          {{ t('knowledge.folder.move.action') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          class="gap-2 whitespace-nowrap text-muted-foreground focus:text-foreground"
          @select="emit('delete', folder)"
        >
          <IconDelete class="size-4" />
          {{ t('knowledge.folder.delete') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>

    <!-- Title: 14px / 22px line-height per design -->
    <p
      class="mx-2 mt-2 line-clamp-2 h-[44px] text-center text-sm font-medium leading-[22px] text-foreground"
      :title="folder.name"
    >
      {{ folder.name }}
    </p>

    <!-- Footer -->
    <div class="mx-2 mt-auto flex items-center justify-between pb-2">
      <div class="flex items-center gap-1 text-xs text-muted-foreground/70">
        <span v-if="documentCount !== undefined">{{ documentCount }}项</span>
        <span v-else>文件夹</span>
      </div>
      <div v-if="latestUpdatedAt" class="text-xs text-muted-foreground/60">
        {{ latestUpdatedAt }}
      </div>
    </div>
  </div>
</template>
