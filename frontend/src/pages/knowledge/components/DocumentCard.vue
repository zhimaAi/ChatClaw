<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  MoreHorizontal,
  FileText,
  Check,
  XCircle,
  AlertTriangle,
  RefreshCw,
  FolderPlus,
} from 'lucide-vue-next'
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
import IconWord from '@/assets/icons/file-word.svg'
import IconExcel from '@/assets/icons/file-excel.svg'
import IconText from '@/assets/icons/file-text.svg'
import IconMarkdown from '@/assets/icons/file-markdown.svg'
import IconHtml from '@/assets/icons/file-html.svg'
import IconCsv from '@/assets/icons/file-csv.svg'
import IconOfd from '@/assets/icons/file-ofd.svg'
import IconDocumentCover from '@/assets/icons/document-cover.svg'
import { DocumentService } from '@bindings/chatclaw/internal/services/document'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

export type DocumentStatus = 'pending' | 'parsing' | 'learning' | 'completed' | 'failed'

export interface Document {
  id: number
  contentHash?: string
  name: string
  fileType: string
  createdAt: string
  // Last updated time of the document (ISO string from backend)
  updatedAt?: string
  // Folder ID this document belongs to (null = uncategorized)
  folderId?: number | null
  status: DocumentStatus
  progress?: number
  thumbIcon?: string // base64 data URI from backend
  errorMessage?: string
  fileMissing?: boolean // 原始文件是否丢失
}

const props = defineProps<{
  document: Document
  isSearching?: boolean
}>()

const emit = defineEmits<{
  (e: 'rename', doc: Document): void
  (e: 'relearn', doc: Document): void
  (e: 'delete', doc: Document): void
  (e: 'move-to-folder', doc: Document): void
  (e: 'detail', doc: Document): void
  (e: 'view', doc: Document): void
  (e: 'navigate-to-folder', doc: Document): void
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
        label: '',
        icon: Check,
        class:
          'size-5 rounded-full bg-black/70 text-white shadow-sm dark:bg-white/15 dark:text-white',
        iconClass: 'text-white',
        iconOnly: true,
        show: true,
      }
    case 'parsing':
      return {
        label: `${props.document.progress || 0}% ${t('knowledge.content.status.parsing')}`,
        icon: null,
        // 半透明黑底 + 白字，表示进行中
        class: 'bg-black/50 text-white dark:bg-white/15 dark:text-white/90',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
    case 'learning':
      return {
        label: `${props.document.progress || 0}% ${t('knowledge.content.status.learning')}`,
        icon: null,
        // 半透明黑底 + 白字，表示进行中
        class: 'bg-black/50 text-white dark:bg-white/15 dark:text-white/90',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
    case 'failed':
      return {
        label: t('knowledge.content.status.failed'),
        icon: XCircle,
        // 浅背景 + 边框 + 深色文字，表示失败/警告
        class: 'bg-background/90 text-foreground/70 ring-1 ring-foreground/20',
        iconClass: 'text-foreground/50',
        iconOnly: false,
        show: true,
      }
    case 'pending':
      return {
        label: t('knowledge.content.status.pending'),
        icon: null,
        // 浅背景 + 边框，表示待处理
        class: 'bg-background/90 text-foreground/60 ring-1 ring-foreground/10',
        iconClass: '',
        iconOnly: false,
        show: true,
      }
    default:
      return {
        label: '',
        icon: null,
        class: '',
        iconClass: '',
        iconOnly: false,
        show: false,
      }
  }
})

const FileIcon = computed(() => {
  const fileType = props.document.fileType?.toLowerCase()
  switch (fileType) {
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
})

// 错误提示显示状态
const showErrorTip = ref(false)
// 文件丢失提示显示状态
const showFileMissingTip = ref(false)

const badgeRef = ref<HTMLElement | null>(null)
const errorTipRef = ref<HTMLElement | null>(null)
const errorTipStyle = ref<Record<string, string>>({})
let hideTimer: number | null = null
let repositionHandler: (() => void) | null = null

const cancelCloseErrorTip = () => {
  if (hideTimer != null) {
    window.clearTimeout(hideTimer)
    hideTimer = null
  }
}

const scheduleCloseErrorTip = () => {
  cancelCloseErrorTip()
  hideTimer = window.setTimeout(() => {
    showErrorTip.value = false
  }, 120)
}

const updateErrorTipPosition = () => {
  const anchor = badgeRef.value
  if (!anchor) return

  const rect = anchor.getBoundingClientRect()
  const padding = 8
  const desiredWidth = 280
  const width = Math.max(180, Math.min(desiredWidth, window.innerWidth - padding * 2))

  let left = rect.left
  if (left + width + padding > window.innerWidth) {
    left = window.innerWidth - width - padding
  }
  if (left < padding) left = padding

  // Prefer showing above; fallback to below if not enough space.
  const preferTop = rect.top > 200 + padding
  if (preferTop) {
    errorTipStyle.value = {
      position: 'fixed',
      left: `${Math.round(left)}px`,
      top: `${Math.round(rect.top - 6)}px`,
      width: `${Math.round(width)}px`,
      transform: 'translateY(-100%)',
    }
  } else {
    errorTipStyle.value = {
      position: 'fixed',
      left: `${Math.round(left)}px`,
      top: `${Math.round(rect.bottom + 6)}px`,
      width: `${Math.round(width)}px`,
      transform: 'translateY(0)',
    }
  }
}

const openErrorTip = async () => {
  if (props.document.status !== 'failed' || !props.document.errorMessage) return
  showErrorTip.value = true
  await nextTick()
  updateErrorTipPosition()
}

const attachRepositionListeners = () => {
  if (repositionHandler) return
  repositionHandler = () => updateErrorTipPosition()
  window.addEventListener('resize', repositionHandler)
  window.addEventListener('scroll', repositionHandler, true)
}

const detachRepositionListeners = () => {
  if (!repositionHandler) return
  window.removeEventListener('resize', repositionHandler)
  window.removeEventListener('scroll', repositionHandler, true)
  repositionHandler = null
}

watch(showErrorTip, async (visible) => {
  if (!visible) {
    detachRepositionListeners()
    return
  }
  await nextTick()
  updateErrorTipPosition()
  attachRepositionListeners()
})

onUnmounted(() => {
  cancelCloseErrorTip()
  detachRepositionListeners()
})

const handleCardClick = () => {
  // Emit view event to open in internal viewer
  emit('view', props.document)
}
</script>

<template>
  <div
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col border border-border bg-card shadow-sm transition-[box-shadow] hover:shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5 dark:hover:ring-white/10"
    @click="handleCardClick"
  >
    <!-- Thumbnail area: 6px radius per design, muted bg #f2f4f7 -->
    <div
      class="relative mx-2 mt-2 h-[86px] w-[150px] overflow-hidden border border-border bg-[#f2f4f7] dark:bg-muted"
    >
      <img
        v-if="document.thumbIcon"
        :src="document.thumbIcon"
        class="size-full object-contain"
        :class="{ 'opacity-50': document.fileMissing }"
        alt=""
      />
      <div v-else class="absolute inset-0 flex items-center justify-center">
        <IconDocumentCover class="size-10 translate-y-1 text-muted-foreground/40" />
      </div>
      <!-- 文件丢失遮罩 -->
      <div
        v-if="document.fileMissing"
        class="absolute inset-0 flex cursor-default items-center justify-center bg-background/60"
        @mouseenter="showFileMissingTip = true"
        @mouseleave="showFileMissingTip = false"
      >
        <AlertTriangle class="size-6 text-muted-foreground" />
      </div>
    </div>

    <!-- 文件丢失浮层提示（放在缩略图区域外面避免被 overflow-hidden 截断） -->
    <div
      v-if="document.fileMissing && showFileMissingTip"
      class="absolute left-1/2 top-[65px] z-50 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-popover px-2 py-1.5 text-xs text-popover-foreground shadow-md"
    >
      {{ t('knowledge.content.fileMissing') }}
    </div>

    <!-- Status badge -->
    <div
      v-if="statusConfig.show"
      ref="badgeRef"
      class="absolute left-3 top-3"
      @mouseenter="
        () => {
          cancelCloseErrorTip()
          openErrorTip()
        }
      "
      @mouseleave="scheduleCloseErrorTip"
    >
      <div
        :class="
          cn(
            statusConfig.iconOnly
              ? 'flex items-center justify-center text-xs'
              : 'flex items-center gap-0.5 rounded px-1.5 py-0.5 text-xs font-medium',
            statusConfig.class
          )
        "
      >
        <component
          :is="statusConfig.icon"
          v-if="statusConfig.icon"
          :class="cn(statusConfig.iconOnly ? 'size-4' : 'size-3.5', statusConfig.iconClass)"
        />
        {{ statusConfig.label }}
      </div>
    </div>

    <!-- 失败原因浮层提示：Teleport 到 body，避免被任何 overflow 裁剪/撑出横向滚动条 -->
    <Teleport to="body">
      <div
        v-if="showErrorTip && document.errorMessage"
        ref="errorTipRef"
        :style="errorTipStyle"
        class="z-9999 max-h-[200px] overflow-y-auto wrap-break-word rounded-md border border-border bg-popover px-2.5 py-2 text-xs leading-relaxed text-popover-foreground shadow-md"
        @mouseenter="cancelCloseErrorTip"
        @mouseleave="scheduleCloseErrorTip"
      >
        {{ document.errorMessage }}
      </div>
    </Teleport>

    <!-- Hover menu -->
    <DropdownMenu>
      <DropdownMenuTrigger
        class="absolute right-2 top-2 flex size-6 items-center justify-center bg-background/80 text-muted-foreground opacity-0 backdrop-blur-sm transition-opacity hover:bg-background hover:text-foreground group-hover:opacity-100"
        @click.stop
      >
        <MoreHorizontal class="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" class="w-auto min-w-fit">
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('detail', document)">
          <FileText class="size-4 text-muted-foreground" />
          {{ t('knowledge.detail.title') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('rename', document)">
          <IconRename class="size-4 text-muted-foreground" />
          {{ t('knowledge.content.menu.rename') }}
        </DropdownMenuItem>
        <DropdownMenuItem class="gap-2 whitespace-nowrap" @select="emit('relearn', document)">
          <RefreshCw class="size-4 text-muted-foreground" />
          {{ t('knowledge.content.menu.relearn') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          class="gap-2 whitespace-nowrap"
          @select="emit('move-to-folder', document)"
        >
          <FolderPlus class="size-4 text-muted-foreground" />
          {{ t('knowledge.content.moveToFolder.title') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="isSearching && document.folderId !== null && document.folderId !== undefined"
          class="gap-2 whitespace-nowrap"
          @select="emit('navigate-to-folder', document)"
        >
          <FolderPlus class="size-4 text-muted-foreground" />
          {{ t('knowledge.content.navigateToFolder') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator
          v-if="isSearching && document.folderId !== null && document.folderId !== undefined"
        />
        <DropdownMenuItem
          class="gap-2 whitespace-nowrap text-muted-foreground focus:text-foreground"
          @select="emit('delete', document)"
        >
          <IconDelete class="size-4" />
          {{ t('knowledge.content.menu.delete') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>

    <!-- Title: 14px / 22px line-height per design -->
    <p
      class="mx-2 mt-2 line-clamp-2 h-[44px] text-left text-sm font-medium leading-[22px] text-foreground"
      :title="document.name"
    >
      {{ document.name }}
    </p>

    <!-- Footer: file type + date -->
    <div class="mx-2 mt-auto flex items-center justify-between pb-2">
      <div class="flex items-center gap-1">
        <component :is="FileIcon" class="size-[14px]" />
        <span class="text-xs text-muted-foreground/70">{{ document.fileType }}</span>
      </div>
      <span class="text-xs text-muted-foreground/60">{{ formatDate(document.createdAt) }}</span>
    </div>
  </div>
</template>
