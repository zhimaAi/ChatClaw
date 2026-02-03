<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MoreHorizontal, FileText, CheckCircle2, Loader2, XCircle } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import IconRename from '@/assets/icons/library-rename.svg'
import IconDelete from '@/assets/icons/library-delete.svg'
import IconPdf from '@/assets/icons/file-pdf.svg'
import IconDocumentCover from '@/assets/icons/document-cover.svg'

export type DocumentStatus = 'pending' | 'learning' | 'completed' | 'failed'

export interface Document {
  id: number
  name: string
  fileType: string
  createdAt: string
  status: DocumentStatus
  progress?: number
  thumbnailUrl?: string
}

const props = defineProps<{
  document: Document
}>()

const emit = defineEmits<{
  (e: 'rename', doc: Document): void
  (e: 'delete', doc: Document): void
}>()

const { t } = useI18n()

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}/${month}/${day}`
}

const statusConfig = computed(() => {
  const status = props.document.status
  switch (status) {
    case 'completed':
      return {
        label: t('knowledge.content.status.completed'),
        icon: CheckCircle2,
        // 深色实心背景 + 白色文字，表示完成
        class: 'bg-foreground/80 text-background',
        iconClass: 'text-background',
        show: true,
      }
    case 'learning':
      return {
        label: `${props.document.progress || 0}% ${t('knowledge.content.status.learning')}`,
        icon: null,
        // 半透明黑底 + 白字，表示进行中
        class: 'bg-black/50 text-white dark:bg-white/15 dark:text-white/90',
        iconClass: '',
        show: true,
      }
    case 'failed':
      return {
        label: t('knowledge.content.status.failed'),
        icon: XCircle,
        // 浅背景 + 边框 + 深色文字，表示失败/警告
        class: 'bg-background/90 text-foreground/70 ring-1 ring-foreground/20',
        iconClass: 'text-foreground/50',
        show: true,
      }
    default:
      return {
        label: '',
        icon: null,
        class: '',
        iconClass: '',
        show: false,
      }
  }
})

const FileIcon = computed(() => {
  const fileType = props.document.fileType?.toLowerCase()
  if (fileType === 'pdf') {
    return IconPdf
  }
  return FileText
})
</script>

<template>
  <div
    class="group relative flex h-[182px] w-[166px] flex-col overflow-hidden rounded-xl border border-border bg-card transition-shadow hover:shadow-md dark:hover:shadow-none dark:hover:ring-1 dark:hover:ring-white/10"
  >
    <!-- 缩略图区域 -->
    <div class="relative mx-[7px] mt-[7px] h-[86px] w-[150px] overflow-hidden rounded-md border border-border bg-muted">
      <div
        v-if="document.thumbnailUrl"
        class="size-full bg-cover bg-center bg-no-repeat"
        :style="{ backgroundImage: `url(${document.thumbnailUrl})` }"
      />
      <div v-else class="flex size-full items-center justify-center">
        <IconDocumentCover class="size-10 text-muted-foreground/40" />
      </div>
    </div>

    <!-- 状态徽章 -->
    <div
      v-if="statusConfig.show"
      :class="cn(
        'absolute left-[11px] top-[11px] flex items-center gap-0.5 rounded px-1.5 py-0.5 text-xs font-medium',
        statusConfig.class
      )"
    >
      <component
        :is="statusConfig.icon"
        v-if="statusConfig.icon"
        :class="cn('size-3.5', statusConfig.iconClass)"
      />
      {{ statusConfig.label }}
    </div>

    <!-- 悬停菜单按钮 -->
    <DropdownMenu>
      <DropdownMenuTrigger
        class="absolute right-[9px] top-[9px] flex size-6 items-center justify-center rounded-md bg-background/80 text-muted-foreground opacity-0 backdrop-blur-sm transition-opacity hover:bg-background hover:text-foreground group-hover:opacity-100"
        @click.stop
      >
        <MoreHorizontal class="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" class="w-32">
        <DropdownMenuItem class="gap-2" @select="emit('rename', document)">
          <IconRename class="size-4 text-muted-foreground" />
          {{ t('knowledge.content.menu.rename') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          class="gap-2 text-destructive focus:text-destructive"
          @select="emit('delete', document)"
        >
          <IconDelete class="size-4" />
          {{ t('knowledge.content.menu.delete') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>

    <!-- 标题 -->
    <p
      class="mx-[7px] mt-[8px] line-clamp-2 h-[44px] text-center text-sm leading-[22px] text-foreground"
      :title="document.name"
    >
      {{ document.name }}
    </p>

    <!-- 底部信息 -->
    <div class="mx-[7px] mt-auto flex items-center justify-between pb-[7px]">
      <div class="flex items-center gap-1">
        <component :is="FileIcon" class="size-[14px]" />
        <span class="text-xs text-muted-foreground/70">{{ document.fileType }}</span>
      </div>
      <span class="text-xs text-muted-foreground/60">{{ formatDate(document.createdAt) }}</span>
    </div>
  </div>
</template>
