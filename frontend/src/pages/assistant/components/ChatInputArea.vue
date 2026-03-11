<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ArrowUp, Square, Check, Lightbulb, X, Image as ImageIcon, FileText, Mic, Video, File } from 'lucide-vue-next'
import { onMounted, onUnmounted } from 'vue'
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
    pendingImages?: PendingImage[]
  }>(),
  { mode: 'assistant', isTeamMode: false, pendingImages: () => [] }
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
const supportsImage = computed(() => {
  return selectedModelCapabilities.value.includes('image')
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
const isDragging = ref(false)

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

const handleSelectImagesClick = () => {
  fileInputRef.value?.click()
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

// Handle paste event on textarea
const handlePaste = async (event: ClipboardEvent) => {
  const items = event.clipboardData?.items
  if (!items) return

  const imageFiles: File[] = []
  
  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) {
        imageFiles.push(file)
      }
    }
  }

  if (imageFiles.length > 0) {
    event.preventDefault() // Prevent pasting image data into textarea
    const validFiles = processImageFiles(imageFiles)
    if (validFiles) {
      emit('addImages', validFiles)
    }
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

  const validFiles = processImageFiles(files)
  if (validFiles) {
    emit('addImages', validFiles)
  }
}

const handleRemoveImage = (id: string) => {
  emit('removeImage', id)
}

// Setup event listeners
onMounted(() => {
  if (textareaRef.value) {
    textareaRef.value.addEventListener('paste', handlePaste)
  }
})

onUnmounted(() => {
  if (textareaRef.value) {
    textareaRef.value.removeEventListener('paste', handlePaste)
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
              class="absolute right-0 top-0 flex size-4 items-center justify-center rounded-bl-md bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100"
              @click="handleRemoveImage(img.id)"
            >
              <X class="size-3" />
            </button>
          </div>
        </div>

        <!-- Selected knowledge bases (hidden in team mode) -->
        <div
          v-if="!isTeamMode && selectedLibraryIds.length > 0"
          class="-mt-1 mb-3 flex flex-wrap items-center gap-1.5"
        >
          <div
            v-for="lib in visibleLibraries"
            :key="lib.id"
            class="group flex items-center gap-1 rounded-md border border-border bg-muted/50 pl-2 pr-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted"
          >
            <span class="max-w-[120px] truncate">{{ lib.name }}</span>
            <button
              class="rounded-sm p-0.5 opacity-0 transition-opacity hover:bg-muted-foreground/10 group-hover:opacity-100"
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

        <div class="mt-3 flex items-center justify-between gap-2">
          <div :class="cn('flex min-w-0 flex-1 flex-wrap items-center', isSnapMode ? 'gap-1' : 'gap-x-2 gap-y-1')">
            <!-- ChatModeSelector: show in both modes -->
            <ChatModeSelector
              v-if="!isTeamMode"
              :model-value="chatMode"
              :compact="isSnapMode"
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
                        class="h-8 w-auto min-w-[100px] max-w-[160px] rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40"
                      >
                        <div v-if="activeAgent" class="flex min-w-0 items-center gap-1.5">
                          <img :src="logoSrc" class="size-3.5 shrink-0" alt="ChatClaw logo" />
                          <span class="truncate">{{ activeAgent.name }}</span>
                        </div>
                        <span v-else class="text-muted-foreground">
                          {{ t('knowledge.chat.selectAgent') }}
                        </span>
                      </SelectTrigger>
                      <SelectContent class="max-h-[260px]">
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

            <!-- Model selector: hidden in team mode -->
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
                          'h-8 w-auto min-w-0 rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40',
                          isSnapMode ? 'max-w-[120px]' : 'max-w-[200px]'
                        )"
                      >
                        <div v-if="selectedModelInfo" class="flex min-w-0 items-center gap-1.5">
                          <ProviderIcon
                            :icon="selectedModelInfo.providerId"
                            :size="14"
                            class="shrink-0 text-foreground"
                          />
                          <span class="truncate">{{ selectedModelInfo.modelName }}</span>
                          <span
                            v-if="selectedProviderIsFree && !isSnapMode"
                            class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                          >
                            {{ t('assistant.chat.freeBadge') }}
                          </span>
                          <template v-if="!isSnapMode && selectedModelCapabilities.length > 0">
                            <span
                              v-for="cap in selectedModelCapabilities.slice(0, 2)"
                              :key="cap"
                              class="shrink-0 rounded px-1 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                            >
                              <component :is="capabilityIcons[cap]" class="size-2.5" />
                            </span>
                          </template>
                        </div>
                        <span v-else class="text-muted-foreground">
                          {{ t('assistant.chat.noModel') }}
                        </span>
                      </SelectTrigger>
                      <SelectContent class="max-h-[260px]">
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
                        ? 'border-primary/50 bg-primary/10 hover:bg-primary/10'
                        : 'hover:bg-muted/40'
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

            <!-- Knowledge base multi-select: hidden in team mode -->
            <SelectRoot
              v-if="!isTeamMode"
              :model-value="selectedLibraryIds"
              multiple
              @update:model-value="(v: any) => { emit('update:selectedLibraryIds', Array.isArray(v) ? v : [v]); handleLibrarySelectionChange() }"
              @update:open="(open: boolean) => open && emit('loadLibraries')"
            >
              <SelectTriggerRaw
                as-child
                :title="
                  selectedLibraryIds.length > 0
                    ? t('assistant.chat.selectedCount', { count: selectedLibraryIds.length })
                    : t('assistant.chat.selectKnowledge')
                "
              >
                <Button
                  size="icon"
                  variant="ghost"
                  class="size-8 rounded-full border border-border bg-background"
                  :class="
                    selectedLibraryIds.length > 0
                      ? 'border-primary/50 bg-primary/10 hover:bg-primary/10'
                      : 'hover:bg-muted/40'
                  "
                >
                  <IconSelectKnowledge
                    class="size-4 pointer-events-none"
                    :class="selectedLibraryIds.length > 0 ? 'text-primary' : 'text-muted-foreground'"
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
                      @click="handleClearLibrarySelection"
                    >
                      {{ t('assistant.chat.clearSelected') }}
                    </div>
                    <SelectSeparator v-if="libraries.length > 0" class="mx-1 my-1 h-px bg-muted" />
                    <!-- Library list -->
                    <template v-if="libraries.length > 0">
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
                    <template v-else>
                      <div class="px-2 py-1.5 text-sm text-muted-foreground">
                        {{ t('assistant.chat.noKnowledge') }}
                      </div>
                    </template>
                  </SelectViewport>
                </SelectContentRaw>
              </SelectPortal>
            </SelectRoot>

            <!-- Image selection button (not supported in team mode) -->
            <TooltipProvider v-if="!isTeamMode">
              <Tooltip>
                <TooltipTrigger as-child>
                  <!-- Wrap in span so tooltip hover still works when button is disabled -->
                  <span class="inline-flex">
                    <Button
                      size="icon"
                      variant="ghost"
                      class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
                      :disabled="!supportsImage"
                      @click="handleSelectImagesClick"
                    >
                      <ImageIcon class="size-4 text-muted-foreground" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{{ supportsImage ? t('assistant.chat.selectImages') : t('assistant.chat.selectImagesDisabled') }}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>


          </div>

          <template v-if="isGenerating">
            <Button
              size="icon"
              class="size-6 rounded-full bg-muted-foreground/20 text-foreground hover:bg-muted-foreground/30"
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
              class="size-6 rounded-full bg-primary text-primary-foreground hover:bg-primary/90"
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
