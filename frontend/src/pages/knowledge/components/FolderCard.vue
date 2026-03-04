<script setup lang="ts">
import { MoreHorizontal } from 'lucide-vue-next'
import { Folder } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
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
    class="group relative flex h-[182px] w-[166px] flex-col rounded-xl border border-border bg-card transition-shadow hover:shadow-md dark:hover:shadow-none dark:hover:ring-1 dark:hover:ring-white/10 cursor-pointer"
    @click="handleCardClick"
  >
    <!-- 文件夹图标区域 -->
    <div
      class="relative mx-[7px] mt-[7px] flex h-[86px] w-[150px] items-center justify-center overflow-hidden rounded-md border border-border bg-muted"
    >
      <Folder class="size-12 text-muted-foreground/60" />
    </div>

    <!-- 悬停菜单按钮 -->
    <DropdownMenu>
      <DropdownMenuTrigger
        class="absolute right-[9px] top-[9px] flex size-6 items-center justify-center rounded-md bg-background/80 text-muted-foreground opacity-0 backdrop-blur-sm transition-opacity hover:bg-background hover:text-foreground group-hover:opacity-100"
        @click.stop
      >
        <MoreHorizontal class="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" class="w-auto min-w-max">
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('rename', folder)">
          <IconRename class="size-4 text-muted-foreground" />
          {{ t('knowledge.folder.rename') }}
        </DropdownMenuItem>
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('move', folder)">
          <FolderPlus class="size-4 text-muted-foreground" />
          {{ t('knowledge.folder.move.title') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          class="gap-2 whitespace-nowrap text-muted-foreground focus:text-foreground"
          @select="emit('delete', folder)"
        >
          <IconDelete class="size-4" />
          {{ t('knowledge.folder.delete') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>

    <!-- 标题 -->
    <p
      class="mx-[7px] mt-[8px] line-clamp-2 h-[44px] text-left text-sm leading-[22px] text-foreground"
      :title="folder.name"
    >
      {{ folder.name }}
    </p>

    <!-- 底部信息 -->
    <div class="mx-[7px] mt-auto flex items-center justify-between pb-[7px]">
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
