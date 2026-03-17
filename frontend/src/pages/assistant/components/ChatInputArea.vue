<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ArrowUp, Square, Check, Lightbulb, X, Image as ImageIcon, FileText, Mic, Video, File, Plus, MoreHorizontal } from 'lucide-vue-next'
import { onMounted, onUnmounted, nextTick } from 'vue'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import {
  SelectRoot,
  SelectTrigger as SelectTriggerRaw,
  SelectPortal,
  SelectContent as SelectContentRaw,
  SelectViewport,
  SelectItem as SelectItemRaw,
  SelectItemIndicator,
  SelectItemText,
  SelectSeparator,
} from 'reka-ui'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import IconSelectKnowledge from '@/assets/icons/select-knowledge.svg'
import ChatModeSelector from './ChatModeSelector.vue'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

import type { ProviderWithModels } from '@bindings/chatclaw/internal/services/providers'
import type { Library } from '@bindings/chatclaw/internal/services/library'
import { useThemeLogo } from '@/composables/useLogo'

interface PendingImage {
  id: string
  file: File
  mimeType: string
  base64: string
  dataUrl: string
  fileName: string
  size: number
}

interface PendingFile {
  id: string
  file: File
  mimeType: string
  base64: string
  fileName: string
  size: number
}

const props = withDefaults(
  defineProps<{
    mode?: 'assistant' | 'knowledge'
    chatInput: string
    chatMode: string
    selectedModelKey: string
    selectedModelInfo: { providerId: string; modelId: string; modelName: string; capabilities?: string[] } | null
    providersWithModels: ProviderWithModels[]
    hasModels: boolean
    enableThinking: boolean
    selectedLibraryIds: number[]
    libraries: Library[]
    isGenerating: boolean
    canSend: boolean
    sendDisabledReason: string
    chatMessages: any[]
    activeAgentId: number | null
    activeAgent: { id: number; name: string } | null
    agents: { id: number; name: string }[]
    isSnapMode?: boolean
    /** Team mode: only plain chat, hide mode/model/thinking/knowledge controls */
    isTeamMode?: boolean
    /** When in knowledge page team tab: current selected team library (display only, like personal selected libraries) */
    selectedTeamLibrary?: { id: string; name: string } | null
    /** When in knowledge page team tab: list of team libraries in current category for the selector dropdown */
    teamLibraries?: { id: string; name: string }[]
    /** When in knowledge page team tab: currently selected team library id (v-model from parent) */
    selectedTeamLibraryId?: string | null
    /** Assistant page (personal list): full team library list when ChatWiki bound; recall uses team_library_id (comma-separated ids) */
    assistantTeamLibraries?: { id: string; name: string }[]
    /** Assistant page: selected team library ids (multi-select; persisted as comma-separated) */
    assistantSelectedTeamLibraryIds?: string[]
    pendingImages?: PendingImage[]
    pendingFiles?: PendingFile[]
  }>(),
  {
    mode: 'assistant',
    isTeamMode: false,
    selectedTeamLibrary: null,
    teamLibraries: () => [],
    selectedTeamLibraryId: null,
    assistantTeamLibraries: () => [],
    assistantSelectedTeamLibraryIds: () => [],
    pendingImages: () => [],
    pendingFiles: () => [],
  }
)

const currentMode = computed(() => props.mode || 'assistant')

const emit = defineEmits<{
  'update:chatInput': [value: string]
  'update:chatMode': [value: string]
  'update:selectedModelKey': [value: string]
  'update:enableThinking': [value: boolean]
  'update:selectedLibraryIds': [value: number[]]
  'update:activeAgentId': [value: number | null]
  send: []
  stop: []
  librarySelectionChange: []
  clearLibrarySelection: []
  loadLibraries: []
  removeLibrary: [id: number]
  addImages: [files: FileList | File[]]
  removeImage: [id: string]
  clearImages: []
  addFiles: [files: FileList | File[]]
  removeFile: [id: string]
  clearFiles: []
  'update:selectedTeamLibraryId': [value: string | null]
  /** Assistant page: toggle one team library id in multi-select (personal + team can coexist) */
  toggleAssistantTeamLibrary: [id: string]
  /** Same as sidebar / header: start a brand new conversation */
  'new-conversation': []
}>()

const { t } = useI18n()
const { logoSrc } = useThemeLogo()

const handleChatEnter = (event: KeyboardEvent) => {
  // Prevent sending when IME is composing (Chinese/Japanese/Korean input).
  // Some browsers report keyCode=229 during composition.

  const anyEvent = event as any
  if (anyEvent?.isComposing || anyEvent?.keyCode === 229) {
    return
  }

  console.warn('[assistant][input] Enter pressed', {
    isTeamMode: props.isTeamMode,
    canSend: props.canSend,
    reason: props.sendDisabledReason,
    chatInputLen: String(props.chatInput ?? '').trim().length,
    activeAgentId: props.activeAgentId,
  })
  event.preventDefault()
  emit('send')
}

const handleSendClick = () => {
  console.warn('[assistant][input] Send button clicked', {
    isTeamMode: props.isTeamMode,
    canSend: props.canSend,
    reason: props.sendDisabledReason,
    chatInputLen: String(props.chatInput ?? '').trim().length,
    activeAgentId: props.activeAgentId,
  })
  emit('send')
}

const handleDisabledSendClick = () => {
  console.warn('[assistant][input] Disabled send clicked', {
    isTeamMode: props.isTeamMode,
    canSend: props.canSend,
    reason: props.sendDisabledReason,
    chatInputLen: String(props.chatInput ?? '').trim().length,
    activeAgentId: props.activeAgentId,
  })
}

const MAX_VISIBLE_LIBRARIES = 3

const selectedLibraries = computed(() =>
  props.libraries.filter((lib) => props.selectedLibraryIds.includes(lib.id))
)

const visibleLibraries = computed(() =>
  selectedLibraries.value.slice(0, MAX_VISIBLE_LIBRARIES)
)

const overflowCount = computed(() =>
  Math.max(0, selectedLibraries.value.length - MAX_VISIBLE_LIBRARIES)
)

/** Team libraries selected for chips (same row as personal) */
const selectedTeamLibraries = computed(() => {
  const ids = props.assistantSelectedTeamLibraryIds || []
  const list = props.assistantTeamLibraries || []
  return ids
    .map((id) => list.find((l) => l.id === id))
    .filter((l): l is { id: string; name: string } => Boolean(l))
})

const MAX_VISIBLE_TEAM = 3
const visibleTeamLibraries = computed(() =>
  selectedTeamLibraries.value.slice(0, MAX_VISIBLE_TEAM)
)
const teamOverflowCount = computed(() =>
  Math.max(0, selectedTeamLibraries.value.length - MAX_VISIBLE_TEAM)
)

// Control knowledge select dropdown open state (so "更多" 菜单可以复用同一套选择逻辑)
const knowledgeSelectOpen = ref(false)

function handleRemoveTeamLibrary(id: string) {
  emit('toggleAssistantTeamLibrary', id)
}

const handleRemoveLibrary = (id: number) => {
  emit('removeLibrary', id)
}

const handleLibrarySelectionChange = () => {
  emit('librarySelectionChange')
}

const handleClearLibrarySelection = () => {
  emit('clearLibrarySelection')
}

// Whether the currently selected model's provider is free (e.g. ChatClaw)
const selectedProviderIsFree = computed(() => {
  if (!props.selectedModelInfo?.providerId || !props.providersWithModels?.length) return false
  const pw = props.providersWithModels.find((p) => p.provider?.provider_id === props.selectedModelInfo?.providerId)
  return isProviderFree(pw)
})

function isProviderFree(pw: ProviderWithModels | undefined): boolean {
  if (!pw?.provider) return false
  const p = pw.provider as { is_free?: boolean }
  return Boolean(p.is_free)
}

// 获取选中模型的能力标签
const selectedModelCapabilities = computed(() => {
  if (props.selectedModelInfo?.capabilities) {
    return props.selectedModelInfo.capabilities
  }
  return []
})

// Whether the currently selected model supports image/vision
// 支持图片识别的模型可以通过调用技能去识别图片，所以不再限制
const supportsImage = computed(() => {
  return true
})

// 能力图标映射
const capabilityIcons: Record<string, any> = {
  text: FileText,
  image: ImageIcon,
  audio: Mic,
  video: Video,
  file: File,
}

const fileInputRef = ref<HTMLInputElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const inputContainerRef = ref<HTMLDivElement | null>(null)
const toolbarRef = ref<HTMLDivElement | null>(null)
const isDragging = ref(false)
const isToolbarNarrow = ref(false)

const MAX_IMAGES = 4
const MAX_IMAGE_SIZE = 2 * 1024 * 1024 // 2MB
const MAX_TOTAL_SIZE = 8 * 1024 * 1024 // 8MB

// Common function to validate and process image files
const processImageFiles = (files: FileList | File[]): File[] | null => {
  if (props.isTeamMode) {
    toast.error(t('assistant.errors.teamImageNotSupported'))
    return null
  }
  const fileArray = Array.from(files)
  
  // Filter only image files
  const imageFiles = fileArray.filter(file => file.type.startsWith('image/'))
  
  if (imageFiles.length === 0) {
    toast.error(t('assistant.errors.invalidImageType'))
    return null
  }

  // Check total count (including existing pending images)
  const currentCount = props.pendingImages.length
  if (currentCount + imageFiles.length > MAX_IMAGES) {
    toast.error(t('assistant.errors.tooManyImages', { max: MAX_IMAGES }))
    return null
  }

  // Validate each file
  let totalSize = props.pendingImages.reduce((sum, img) => sum + img.size, 0)
  
  for (const file of imageFiles) {
    if (file.size > MAX_IMAGE_SIZE) {
      toast.error(t('assistant.errors.imageTooLarge', { max: '2MB' }))
      return null
    }
    totalSize += file.size
  }

  if (totalSize > MAX_TOTAL_SIZE) {
    toast.error(t('assistant.errors.imagesTotalTooLarge', { max: '8MB' }))
    return null
  }

  return imageFiles
}

const formatFileSize = (bytes: number): string => {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

const handleSelectImagesClick = () => {
  fileInputRef.value?.click()
}

const docFileInputRef = ref<HTMLInputElement | null>(null)

const handleSelectFilesClick = () => {
  docFileInputRef.value?.click()
}

const handleDocFilesSelected = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0) return

  emit('addFiles', Array.from(files))

  if (docFileInputRef.value) {
    docFileInputRef.value.value = ''
  }
}

const handleFilesSelected = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0) return

  const validFiles = processImageFiles(files)
  if (validFiles) {
    emit('addImages', validFiles)
  }
  
  // Reset input so same file can be selected again
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
  }
}

const ALLOWED_DOC_EXTENSIONS = new Set([
  'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx',
  'txt', 'csv', 'md', 'json', 'xml', 'html', 'rtf', 'log',
])

// Handle paste event on textarea
const handlePaste = async (event: ClipboardEvent) => {
  const items = event.clipboardData?.items
  if (!items) return

  const imageFiles: File[] = []
  const docFiles: File[] = []

  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    const file = item.getAsFile()
    if (!file) continue

    if (item.type.startsWith('image/')) {
      imageFiles.push(file)
    } else {
      const ext = file.name?.split('.').pop()?.toLowerCase() || ''
      if (ALLOWED_DOC_EXTENSIONS.has(ext)) {
        docFiles.push(file)
      }
    }
  }

  const hasAttachments = imageFiles.length > 0 || docFiles.length > 0
  if (!hasAttachments) return

  event.preventDefault()

  if (imageFiles.length > 0) {
    const validFiles = processImageFiles(imageFiles)
    if (validFiles) {
      emit('addImages', validFiles)
    }
  }

  if (docFiles.length > 0) {
    emit('addFiles', docFiles)
  }
}

// Handle drag and drop events
const handleDragOver = (event: DragEvent) => {
  event.preventDefault()
  event.stopPropagation()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy'
    isDragging.value = true
  }
}

const handleDragLeave = (event: DragEvent) => {
  event.preventDefault()
  event.stopPropagation()
  // Only set isDragging to false if we're leaving the container
  const relatedTarget = event.relatedTarget as HTMLElement
  if (!inputContainerRef.value?.contains(relatedTarget)) {
    isDragging.value = false
  }
}

const handleDrop = async (event: DragEvent) => {
  event.preventDefault()
  event.stopPropagation()
  isDragging.value = false

  const files = event.dataTransfer?.files
  if (!files || files.length === 0) return

  const fileArray = Array.from(files)
  const imageFiles = fileArray.filter(f => f.type.startsWith('image/'))
  const docFiles = fileArray.filter(f => {
    if (f.type.startsWith('image/')) return false
    const ext = f.name.split('.').pop()?.toLowerCase() || ''
    return ALLOWED_DOC_EXTENSIONS.has(ext)
  })

  if (imageFiles.length > 0) {
    const validImages = processImageFiles(imageFiles)
    if (validImages) {
      emit('addImages', validImages)
    }
  }

  if (docFiles.length > 0) {
    emit('addFiles', docFiles)
  }
}

const handleRemoveImage = (id: string) => {
  emit('removeImage', id)
}

// Whether to collapse file/image/knowledge buttons into a "More" dropdown
const useCompactToolbar = computed(() => props.isSnapMode || isToolbarNarrow.value)

// ResizeObserver to detect narrow toolbar
let toolbarObserver: ResizeObserver | null = null

// Setup event listeners
onMounted(() => {
  if (textareaRef.value) {
    textareaRef.value.addEventListener('paste', handlePaste)
  }

  nextTick(() => {
    if (toolbarRef.value) {
      toolbarObserver = new ResizeObserver((entries) => {
        for (const entry of entries) {
          // Threshold: when toolbar width is below ~480px, collapse extra buttons
          isToolbarNarrow.value = entry.contentRect.width < 480
        }
      })
      toolbarObserver.observe(toolbarRef.value)
    }
  })
})

onUnmounted(() => {
  if (textareaRef.value) {
    textareaRef.value.removeEventListener('paste', handlePaste)
  }
  if (toolbarObserver) {
    toolbarObserver.disconnect()
    toolbarObserver = null
  }
})
</script>

<template>
  <div
    :class="
      cn(
        'flex px-6',
        currentMode === 'assistant' && chatMessages.length === 0 && !isGenerating
          ? 'flex-1 items-center justify-center'
          : 'pb-4'
      )
    "
  >
    <div
      :class="
        cn(
          'flex w-full flex-col items-center gap-10',
          chatMessages.length === 0 && !isGenerating && '-translate-y-10'
        )
      "
    >
      <div
        v-if="currentMode === 'assistant' && chatMessages.length === 0 && !isGenerating"
        class="flex items-center gap-3"
      >
        <img :src="logoSrc" class="size-10" alt="ChatClaw logo" />
        <div class="text-2xl font-semibold text-foreground">
          {{ t('app.title') }}
        </div>
      </div>

      <div
        ref="inputContainerRef"
        :class="cn(
          'w-full max-w-[800px] rounded-2xl border border-border bg-background px-4 pt-4 pb-3 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10',
          isDragging && 'ring-2 ring-primary/50 border-primary/50',
          currentMode === 'knowledge' && 'border-t'
        )"
        @dragover="handleDragOver"
        @dragleave="handleDragLeave"
        @drop="handleDrop"
      >
        <!-- Image preview area -->
        <div v-if="pendingImages.length > 0" class="-mt-1 mb-3 flex flex-wrap gap-2">
          <div
            v-for="img in pendingImages"
            :key="img.id"
            class="group relative h-16 w-16 overflow-hidden rounded-md border border-border bg-muted/40"
          >
            <img :src="img.dataUrl" class="h-full w-full object-cover" :alt="img.fileName" />
            <button
              class="absolute right-0 top-0 flex size-4 cursor-pointer items-center justify-center rounded-bl-md bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100 active:bg-destructive"
              @click="handleRemoveImage(img.id)"
            >
              <X class="size-3" />
            </button>
          </div>
        </div>

        <!-- File preview area -->
        <div v-if="pendingFiles.length > 0" class="-mt-1 mb-3 flex flex-wrap gap-2">
          <div
            v-for="f in pendingFiles"
            :key="f.id"
            class="group relative flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2"
          >
            <File class="size-4 shrink-0 text-muted-foreground" />
            <div class="flex min-w-0 flex-col">
              <span class="truncate text-xs font-medium text-foreground" :title="f.fileName">{{ f.fileName }}</span>
              <span class="text-[10px] text-muted-foreground">{{ formatFileSize(f.size) }}</span>
            </div>
            <button
              class="ml-1 flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-full bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100 active:bg-destructive"
              @click="emit('removeFile', f.id)"
            >
              <X class="size-3" />
            </button>
          </div>
        </div>

        <!-- Selected knowledge bases: personal + team chips (both removable; same style) -->
        <div
          v-if="
            !isTeamMode &&
            (selectedLibraryIds.length > 0 ||
              (assistantSelectedTeamLibraryIds && assistantSelectedTeamLibraryIds.length > 0))
          "
          class="-mt-1 mb-3 flex flex-wrap items-center gap-1.5"
        >
          <!-- Personal libraries -->
          <div
            v-for="lib in visibleLibraries"
            :key="'p-' + lib.id"
            class="group flex items-center gap-1 rounded-md border border-border bg-muted/50 pl-2 pr-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted"
          >
            <span class="max-w-[120px] truncate">{{ lib.name }}</span>
            <button
              class="cursor-pointer rounded-sm p-0.5 opacity-0 transition-opacity hover:bg-muted-foreground/10 active:bg-muted-foreground/20 group-hover:opacity-100"
              @click="handleRemoveLibrary(lib.id)"
            >
              <X class="size-3" />
            </button>
          </div>
          <span
            v-if="overflowCount > 0"
            class="rounded-md border border-border bg-muted/50 px-2 py-0.5 text-xs text-muted-foreground"
          >
            +{{ overflowCount }}
          </span>
          <!-- Team libraries (ChatWiki) -->
          <div
            v-for="lib in visibleTeamLibraries"
            :key="'t-' + lib.id"
            class="group flex items-center gap-1 rounded-md border border-border bg-muted/50 pl-2 pr-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted"
            :title="lib.name"
          >
            <span class="max-w-[120px] truncate">{{ lib.name }}</span>
            <button
              class="cursor-pointer rounded-sm p-0.5 opacity-0 transition-opacity hover:bg-muted-foreground/10 active:bg-muted-foreground/20 group-hover:opacity-100"
              @click="handleRemoveTeamLibrary(lib.id)"
            >
              <X class="size-3" />
            </button>
          </div>
          <span
            v-if="teamOverflowCount > 0"
            class="rounded-md border border-border bg-muted/50 px-2 py-0.5 text-xs text-muted-foreground"
          >
            +{{ teamOverflowCount }}
          </span>
        </div>
        <div
          v-else-if="selectedTeamLibrary"
          class="-mt-1 mb-3 flex flex-wrap items-center gap-1.5"
        >
          <div
            class="flex items-center gap-1 rounded-md border border-border bg-muted/50 pl-2 pr-2 py-0.5 text-xs text-muted-foreground"
            :title="selectedTeamLibrary.name"
          >
            <span class="max-w-[160px] truncate">{{ selectedTeamLibrary.name }}</span>
          </div>
        </div>

        <textarea
          ref="textareaRef"
          :value="chatInput"
          :placeholder="t('assistant.placeholders.inputPlaceholder')"
          class="min-h-[64px] w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          rows="2"
          @input="emit('update:chatInput', ($event.target as HTMLTextAreaElement).value)"
          @keydown.enter.exact="handleChatEnter"
        />

        <div ref="toolbarRef" class="mt-3 flex items-center justify-between gap-2">
          <div
            :class="
              cn(
                'flex min-w-0 flex-1 items-center',
                useCompactToolbar ? 'flex-nowrap gap-1' : 'flex-wrap gap-x-2 gap-y-1'
              )
            "
          >
            <!-- New conversation icon：行为与侧边栏 / 吸附头部一致 -->
            <TooltipProvider v-if="!isTeamMode">
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-8 rounded-full border border-border bg-background hover:bg-muted/40 active:bg-muted active:scale-95"
                    @click="emit('new-conversation')"
                  >
                    <Plus class="size-4 text-muted-foreground" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ t('assistant.sidebar.newConversation') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <!-- ChatModeSelector: show in both modes，内部自己处理悬浮提示 -->
            <ChatModeSelector
              v-if="!isTeamMode"
              :model-value="chatMode"
              :compact="true"
              @update:model-value="(v) => emit('update:chatMode', v)"
            />

            <!-- Agent selector: only show in knowledge mode -->
            <TooltipProvider v-if="currentMode === 'knowledge'">
              <Tooltip>
                <TooltipTrigger as-child>
                  <div class="min-w-0">
                    <Select
                      :model-value="activeAgentId != null ? String(activeAgentId) : undefined"
                      :disabled="agents.length === 0"
                      @update:model-value="(v: any) => v && emit('update:activeAgentId', Number(v))"
                    >
                      <SelectTrigger
                        class="h-8 w-auto min-w-[100px] max-w-[160px] cursor-pointer rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40 active:bg-muted active:scale-95"
                      >
                        <div v-if="activeAgent" class="flex min-w-0 items-center gap-1.5">
                          <img :src="logoSrc" class="size-3.5 shrink-0" alt="ChatClaw logo" />
                          <span class="truncate">{{ activeAgent.name }}</span>
                        </div>
                        <span v-else class="text-muted-foreground">
                          {{ t('knowledge.chat.selectAgent') }}
                        </span>
                      </SelectTrigger>
                      <SelectContent class="max-h-[260px] min-w-[260px]">
                        <SelectGroup>
                          <SelectLabel>{{ t('knowledge.chat.selectAgent') }}</SelectLabel>
                          <SelectItem
                            v-for="a in agents"
                            :key="a.id"
                            :value="String(a.id)"
                          >
                            {{ a.name }}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>
                </TooltipTrigger>
                <TooltipContent v-if="activeAgent">
                  <p>{{ activeAgent.name }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <!-- Model selector: hidden in team mode，展示完整模型名的胶囊按钮 -->
            <div class="min-w-0 shrink">
            <TooltipProvider v-if="!isTeamMode">
              <Tooltip>
                <TooltipTrigger as-child>
                  <div class="min-w-0">
                    <Select
                      :model-value="selectedModelKey"
                      :disabled="!hasModels"
                      @update:model-value="(v: any) => v && emit('update:selectedModelKey', String(v))"
                    >
                      <SelectTrigger
                        :class="cn(
                          'h-8 w-full min-w-0 max-w-[220px] cursor-pointer rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40 active:bg-muted active:scale-95',
                          useCompactToolbar && 'max-w-[140px]'
                        )"
                      >
                        <div v-if="selectedModelInfo" class="flex min-w-0 items-center gap-1.5">
                          <ProviderIcon
                            :icon="selectedModelInfo.providerId"
                            :size="14"
                            class="shrink-0 text-foreground"
                          />
                          <span class="truncate">{{ selectedModelInfo.modelName }}</span>
                        </div>
                        <span v-else class="text-muted-foreground">
                          {{ t('assistant.chat.noModel') }}
                        </span>
                      </SelectTrigger>
                      <SelectContent class="max-h-[260px] min-w-[260px]">
                        <SelectGroup>
                          <SelectLabel>{{ t('assistant.chat.selectModel') }}</SelectLabel>
                          <template v-for="pw in providersWithModels" :key="pw.provider.provider_id">
                            <SelectLabel class="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
                              <span>{{ pw.provider.name }}</span>
                              <span
                                v-if="isProviderFree(pw)"
                                class="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                              >
                                {{ t('assistant.chat.freeBadge') }}
                              </span>
                            </SelectLabel>
                            <template v-for="g in pw.model_groups" :key="g.type">
                              <template v-if="g.type === 'llm'">
                                <SelectItem
                                  v-for="m in g.models"
                                  :key="pw.provider.provider_id + '::' + m.model_id"
                                  :value="pw.provider.provider_id + '::' + m.model_id"
                                >
                                  <div class="flex items-center gap-2">
                                    <span>{{ m.name }}</span>
                                    <template v-if="m.capabilities && m.capabilities.length > 0">
                                      <span
                                        v-for="cap in m.capabilities.slice(0, 3)"
                                        :key="cap"
                                        class="rounded px-1 py-0.5 text-[10px] text-muted-foreground"
                                      >
                                        <component :is="capabilityIcons[cap]" class="size-2.5" />
                                      </span>
                                    </template>
                                  </div>
                                </SelectItem>
                              </template>
                            </template>
                          </template>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </div>
                </TooltipTrigger>
                <TooltipContent v-if="selectedModelInfo">
                  <p>{{ selectedModelInfo.modelName }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            </div>

            <!-- Thinking mode toggle: hidden in team mode -->
            <TooltipProvider v-if="!isTeamMode">
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-8 rounded-full border border-border bg-background"
                    :class="
                      enableThinking
                        ? 'border-primary/50 bg-primary/10 hover:bg-primary/10 active:bg-primary/20 active:scale-95'
                        : 'hover:bg-muted/40 active:bg-muted active:scale-95'
                    "
                    @click="emit('update:enableThinking', !enableThinking)"
                  >
                    <Lightbulb
                      class="size-4 pointer-events-none"
                      :class="enableThinking ? 'text-primary' : 'text-muted-foreground'"
                    />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ enableThinking ? t('assistant.chat.thinkingOn') : t('assistant.chat.thinkingOff') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <input
              ref="fileInputRef"
              type="file"
              accept="image/*"
              multiple
              class="hidden"
              @change="handleFilesSelected"
            />
            <input
              ref="docFileInputRef"
              type="file"
              accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.md,.json,.xml,.html,.rtf,.log"
              multiple
              class="hidden"
              @change="handleDocFilesSelected"
            />

            <!-- Knowledge base select: same slot for personal (multi-select) and team (single from current category); position unchanged -->
            <SelectRoot
              v-if="!isTeamMode && !selectedTeamLibrary"
              :model-value="selectedLibraryIds"
              :open="knowledgeSelectOpen"
              multiple
              @update:model-value="(v: any) => { emit('update:selectedLibraryIds', Array.isArray(v) ? v : [v]); handleLibrarySelectionChange() }"
              @update:open="(open: boolean) => { knowledgeSelectOpen = open; open && emit('loadLibraries') }"
            >
              <SelectTriggerRaw
                as-child
                :title="
                  assistantSelectedTeamLibraryIds && assistantSelectedTeamLibraryIds.length > 0
                    ? assistantSelectedTeamLibraryIds.length === 1
                      ? (assistantTeamLibraries.find((l) => l.id === assistantSelectedTeamLibraryIds[0])?.name ?? '')
                      : t('assistant.chat.selectedCount', { count: assistantSelectedTeamLibraryIds.length })
                    : selectedLibraryIds.length > 0
                      ? t('assistant.chat.selectedCount', { count: selectedLibraryIds.length })
                      : t('assistant.chat.selectKnowledge')
                "
              >
                <Button
                  size="icon"
                  variant="ghost"
                  :class="
                    cn(
                      'size-8 rounded-full border border-border bg-background',
                      (assistantSelectedTeamLibraryIds && assistantSelectedTeamLibraryIds.length > 0) || selectedLibraryIds.length > 0
                        ? 'border-primary/50 bg-primary/10 hover:bg-primary/10 active:bg-primary/20 active:scale-95'
                        : 'hover:bg-muted/40 active:bg-muted active:scale-95',
                      useCompactToolbar && 'w-0 p-0 border-none bg-transparent shadow-none overflow-hidden'
                    )
                  "
                >
                  <IconSelectKnowledge
                    class="size-4 pointer-events-none"
                    :class="(assistantSelectedTeamLibraryIds && assistantSelectedTeamLibraryIds.length > 0) || selectedLibraryIds.length > 0 ? 'text-primary' : 'text-muted-foreground'"
                  />
                </Button>
              </SelectTriggerRaw>
              <SelectPortal>
                <SelectContentRaw
                  class="z-50 max-h-[300px] min-w-[200px] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
                  position="popper"
                  :side-offset="5"
                >
                  <SelectViewport>
                    <!-- Clear selection option - use a div with click handler since SelectItem would add it to selection -->
                    <div
                      class="relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm text-muted-foreground outline-none hover:bg-accent hover:text-accent-foreground"
                      @click.stop="handleClearLibrarySelection"
                    >
                      {{ t('assistant.chat.clearSelected') }}
                    </div>
                    <SelectSeparator v-if="libraries.length > 0 || (assistantTeamLibraries && assistantTeamLibraries.length > 0)" class="mx-1 my-1 h-px bg-muted" />
                    <!-- Personal libraries (multi-select) -->
                    <template v-if="libraries.length > 0">
                      <div class="px-2 py-1 text-[10px] font-medium uppercase text-muted-foreground">
                        {{ t('assistant.chat.personalKnowledgeSection') }}
                      </div>
                      <SelectItemRaw
                        v-for="lib in libraries"
                        :key="lib.id"
                        :value="Number(lib.id)"
                        class="relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50"
                      >
                        <SelectItemIndicator
                          class="absolute left-2 flex size-4 items-center justify-center"
                        >
                          <Check class="size-4 text-primary" />
                        </SelectItemIndicator>
                        <SelectItemText>{{ lib.name }}</SelectItemText>
                      </SelectItemRaw>
                    </template>
                    <template v-else-if="!(assistantTeamLibraries && assistantTeamLibraries.length > 0)">
                      <div class="px-2 py-1.5 text-sm text-muted-foreground">
                        {{ t('assistant.chat.noKnowledge') }}
                      </div>
                    </template>
                    <!-- ChatWiki team libraries: multi-select like personal; recall id = comma-separated -->
                    <template v-if="assistantTeamLibraries && assistantTeamLibraries.length > 0">
                      <SelectSeparator v-if="libraries.length > 0" class="mx-1 my-1 h-px bg-muted" />
                      <div class="px-2 py-1 text-[10px] font-medium uppercase text-muted-foreground">
                        {{ t('assistant.chat.chatwikiSection') }}
                      </div>
                      <div
                        v-for="lib in assistantTeamLibraries"
                        :key="`team-${lib.id}`"
                        class="relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none hover:bg-accent hover:text-accent-foreground data-highlighted:bg-accent data-highlighted:text-accent-foreground"
                        @click.stop="() => emit('toggleAssistantTeamLibrary', lib.id)"
                      >
                        <span
                          v-if="assistantSelectedTeamLibraryIds && assistantSelectedTeamLibraryIds.includes(lib.id)"
                          class="absolute left-2 flex size-4 items-center justify-center"
                        >
                          <Check class="size-4 text-primary" />
                        </span>
                        <span class="pl-0">{{ lib.name }}</span>
                      </div>
                    </template>
                  </SelectViewport>
                </SelectContentRaw>
              </SelectPortal>
            </SelectRoot>

            <!-- Team tab: same icon position as personal, opens current category team library list.
                 Rendered whenever selectedTeamLibrary is set; disabled when list is empty so the
                 icon stays visible and the user can see which library is active. -->
            <template v-else-if="selectedTeamLibrary">
              <SelectRoot
                v-if="teamLibraries && teamLibraries.length > 0"
                :model-value="selectedTeamLibraryId ?? undefined"
                @update:model-value="(v: string | undefined) => emit('update:selectedTeamLibraryId', v ?? null)"
              >
                <SelectTriggerRaw
                  as-child
                  :title="selectedTeamLibrary.name"
                >
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-8 rounded-full border border-border bg-background border-primary/50 bg-primary/10 hover:bg-primary/10 active:bg-primary/20 active:scale-95"
                  >
                    <IconSelectKnowledge class="size-4 pointer-events-none text-primary" />
                  </Button>
                </SelectTriggerRaw>
                <SelectPortal>
                  <SelectContentRaw
                    class="z-50 max-h-[300px] min-w-[200px] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
                    position="popper"
                    :side-offset="5"
                  >
                    <SelectViewport>
                      <SelectItemRaw
                        v-for="lib in teamLibraries"
                        :key="lib.id"
                        :value="lib.id"
                        class="relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground"
                      >
                        <SelectItemIndicator
                          class="absolute left-2 flex size-4 items-center justify-center"
                        >
                          <Check class="size-4 text-primary" />
                        </SelectItemIndicator>
                        <SelectItemText>{{ lib.name }}</SelectItemText>
                      </SelectItemRaw>
                    </SelectViewport>
                  </SelectContentRaw>
                </SelectPortal>
              </SelectRoot>
              <!-- Library list not yet loaded: show icon in active state but non-interactive -->
              <Button
                v-else
                size="icon"
                variant="ghost"
                disabled
                :title="selectedTeamLibrary.name"
                class="size-8 rounded-full border border-border bg-background border-primary/50 bg-primary/10"
              >
                <IconSelectKnowledge class="size-4 pointer-events-none text-primary" />
              </Button>
            </template>

            <!-- File upload button (wide toolbar only) -->
            <TooltipProvider v-if="!isTeamMode && !useCompactToolbar">
              <Tooltip>
                <TooltipTrigger as-child>
                  <span class="inline-flex">
                    <Button
                      size="icon"
                      variant="ghost"
                      class="size-8 rounded-full border border-border bg-background hover:bg-muted/40 active:bg-muted active:scale-95"
                      @click="handleSelectFilesClick"
                    >
                      <File class="size-4 text-muted-foreground" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ t('assistant.chat.uploadFile') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <!-- Image selection button (non-snap mode).
                 在窗口较宽时直接展示图标；在 snap 模式下收纳到“更多”菜单中。 -->
            <TooltipProvider v-if="!isTeamMode && !useCompactToolbar">
              <Tooltip>
                <TooltipTrigger as-child>
                  <span class="inline-flex">
                    <Button
                      size="icon"
                      variant="ghost"
                      class="size-8 rounded-full border border-border bg-background hover:bg-muted/40 active:bg-muted active:scale-95"
                      @click="handleSelectImagesClick"
                    >
                      <ImageIcon class="size-4 text-muted-foreground" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ t('assistant.chat.selectImages') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>

            <!-- 更多：在空间较小时，将“上传文件 / 上传图片 / 选择知识库”放进菜单 -->
            <DropdownMenu v-if="!isTeamMode && useCompactToolbar">
              <DropdownMenuTrigger as-child>
                <Button
                  size="icon"
                  variant="ghost"
                  class="size-8 rounded-full border border-border bg-background hover:bg-muted/40 active:bg-muted active:scale-95"
                >
                  <MoreHorizontal class="size-4 text-muted-foreground" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" class="w-40 rounded-[6px] shadow-[0_6px_30px_rgba(0,0,0,0.05),0_16px_24px_rgba(0,0,0,0.04),0_8px_10px_rgba(0,0,0,0.08)]">
                <DropdownMenuItem class="gap-2" @select="handleSelectFilesClick">
                  <File class="size-4 text-muted-foreground" />
                  <span class="text-xs text-foreground">{{ t('assistant.chat.uploadFile') }}</span>
                </DropdownMenuItem>
                <DropdownMenuItem class="gap-2" @select="handleSelectImagesClick">
                  <ImageIcon class="size-4 text-muted-foreground" />
                  <span class="text-xs text-foreground">{{ t('assistant.chat.selectImages') }}</span>
                </DropdownMenuItem>
                <DropdownMenuItem
                  class="gap-2"
                  @select="() => { knowledgeSelectOpen = true; emit('loadLibraries') }"
                >
                  <IconSelectKnowledge class="size-4 text-muted-foreground" />
                  <span class="text-xs text-foreground">{{ t('assistant.chat.selectKnowledge') }}</span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

          </div>

          <template v-if="isGenerating">
            <Button
              size="icon"
              class="size-6 rounded-full bg-muted-foreground/20 text-foreground hover:bg-muted-foreground/30 active:bg-muted-foreground/50 active:scale-95"
              :title="t('assistant.chat.stop')"
              @click="emit('stop')"
            >
              <Square class="size-4" />
            </Button>
          </template>
          <template v-else>
            <TooltipProvider v-if="!canSend">
              <Tooltip>
                <TooltipTrigger as-child>
                  <!-- disabled button has pointer-events-none; use wrapper to keep tooltip hover -->
                  <span class="inline-flex" @click="handleDisabledSendClick">
                    <Button
                      size="icon"
                      class="size-6 pointer-events-none rounded-full bg-muted-foreground/20 text-muted-foreground disabled:opacity-100"
                      disabled
                    >
                      <ArrowUp class="size-4" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ sendDisabledReason || t('assistant.placeholders.enterToSend') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <Button
              v-else
              size="icon"
              class="size-6 rounded-full bg-primary text-primary-foreground hover:bg-primary/90 active:bg-primary/75 active:scale-95"
              :title="t('assistant.chat.send')"
              @click="handleSendClick"
            >
              <ArrowUp class="size-4" />
            </Button>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
