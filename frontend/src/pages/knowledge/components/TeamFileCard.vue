<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MoreHorizontal, Check, FileText } from 'lucide-vue-next'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import IconPdf from '@/assets/icons/file-pdf.svg'
import IconWord from '@/assets/icons/file-word.svg'
import IconExcel from '@/assets/icons/file-excel.svg'
import IconText from '@/assets/icons/file-text.svg'
import IconMarkdown from '@/assets/icons/file-markdown.svg'
import IconHtml from '@/assets/icons/file-html.svg'
import IconCsv from '@/assets/icons/file-csv.svg'
import IconOfd from '@/assets/icons/file-ofd.svg'

const { t } = useI18n()

interface TeamFile {
  id: string
  name: string
  extension: string
  status: number
  [key: string]: any
}

const props = defineProps<{
  file: TeamFile
}>()

const emit = defineEmits<{
  (e: 'click', file: TeamFile): void
}>()

const formatDate = (dateStr: string) => {
  if (!dateStr) return ''
  let date: Date
  // 检查是否是 Unix 时间戳（纯数字字符串）
  if (/^\d+$/.test(dateStr)) {
    // Unix 时间戳可能是秒或毫秒
    const timestamp = parseInt(dateStr, 10)
    // 如果小于 10 位数，可能是秒级时间戳，转换为毫秒
    date = timestamp < 10000000000 ? new Date(timestamp * 1000) : new Date(timestamp)
  } else {
    date = new Date(dateStr)
  }
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}/${month}/${day}`
}

const getDate = () => {
  // 支持多种可能的字段名
  return (
    props.file.updatedAt ||
    props.file.updated_at ||
    props.file.updated_at_unix ||
    props.file.updateTime ||
    props.file.updatedTime ||
    ''
  )
}

const getFileExtension = (file: TeamFile) => {
  const ext = String(file.extension || '')
    .trim()
    .toLowerCase()
  if (ext) return ext
  const name = String(file.name || '')
  const idx = name.lastIndexOf('.')
  if (idx < 0 || idx === name.length - 1) return ''
  return name
    .slice(idx + 1)
    .trim()
    .toLowerCase()
}

const getFileIcon = (file: TeamFile) => {
  const ext = getFileExtension(file)
  switch (ext) {
    case 'pdf':
      return IconPdf
    case 'doc':
    case 'docx':
      return IconWord
    case 'xls':
    case 'xlsx':
      return IconExcel
    case 'txt':
      return IconText
    case 'md':
    case 'markdown':
      return IconMarkdown
    case 'html':
    case 'htm':
      return IconHtml
    case 'csv':
      return IconCsv
    case 'ofd':
      return IconOfd
    default:
      return FileText
  }
}

const getFileStatusLabel = (status: number) => {
  if (status === 3) return t('knowledge.content.status.failed')
  if (status === 2) return ''
  if (status === 1) return t('knowledge.content.status.learning')
  return t('knowledge.content.status.pending')
}

const statusConfig = computed(() => {
  const status = props.file.status
  switch (status) {
    case 2:
      return {
        label: '',
        icon: Check,
        class:
          'size-5 rounded-full bg-black/70 text-white shadow-sm dark:bg-white/15 dark:text-white',
        iconClass: 'text-white',
        iconOnly: true,
        show: true,
      }
    case 1:
      return {
        label: t('knowledge.content.status.learning'),
        icon: null,
        class: 'bg-black/50 text-white dark:bg-white/15 dark:text-white/90',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
    case 3:
      return {
        label: t('knowledge.content.status.failed'),
        icon: null,
        class: 'bg-background/90 text-foreground/70 ring-1 ring-foreground/20',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
    case 0:
    default:
      return {
        label: t('knowledge.content.status.pending'),
        icon: null,
        class: 'bg-background/90 text-foreground/60 ring-1 ring-foreground/10',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
  }
})

const fileThumbUrl = computed(() => {
  // 支持多种可能的字段名
  return (
    props.file.thumbPath || props.file.thumb_path || props.file.thumb || props.file.thumbnail || ''
  )
})

const canShowThumb = computed(() => {
  return !!fileThumbUrl.value
})

const handleCardClick = () => {
  emit('click', props.file)
}
</script>

<template>
  <div
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col rounded-xl border border-border bg-card transition-shadow hover:shadow-md dark:hover:shadow-none dark:hover:ring-1 dark:hover:ring-white/10"
    @click="handleCardClick"
  >
    <!-- 缩略图区域 -->
    <div
      class="relative mx-[7px] mt-[7px] h-[86px] w-[150px] overflow-hidden rounded-md border border-border bg-muted"
    >
      <img v-if="canShowThumb" :src="fileThumbUrl" class="size-full object-contain" alt="" />
      <div v-else class="absolute inset-0 flex items-center justify-center">
        <component :is="getFileIcon(file)" class="size-10 text-muted-foreground/40" />
      </div>
    </div>

    <!-- 状态徽章 -->
    <div
      v-if="statusConfig.show && statusConfig.iconOnly"
      class="absolute left-[11px] top-[11px] flex items-center justify-center text-xs"
      :class="statusConfig.class"
    >
      <component
        :is="statusConfig.icon"
        v-if="statusConfig.icon"
        class="size-4"
        :class="statusConfig.iconClass"
      />
    </div>
    <div
      v-else-if="statusConfig.show"
      class="absolute left-[11px] top-[11px] flex items-center gap-0.5 rounded px-1.5 py-0.5 text-xs font-medium"
      :class="statusConfig.class"
    >
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
      <DropdownMenuContent align="end" class="w-auto min-w-fit">
        <DropdownMenuItem class="gap-2 whitespace-nowrap">
          <FileText class="size-4 text-muted-foreground" />
          {{ t('knowledge.detail.title') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>

    <!-- 标题 -->
    <p
      class="mx-[7px] mt-[8px] line-clamp-2 h-[44px] text-left text-sm leading-[22px] text-foreground"
      :title="file.name"
    >
      {{ file.name }}
    </p>

    <!-- 底部信息 -->
    <div class="mx-[7px] mt-auto flex items-center justify-between pb-[7px]">
      <div class="flex items-center gap-1">
        <component :is="getFileIcon(file)" class="size-[14px]" />
        <span class="text-xs text-muted-foreground/70">{{ getFileExtension(file) || '-' }}</span>
      </div>
      <span class="text-xs text-muted-foreground/60">{{ formatDate(getDate()) }}</span>
    </div>
  </div>
</template>
