<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ArrowUp, Square, Check, Lightbulb } from 'lucide-vue-next'
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

import type { ProviderWithModels } from '@bindings/willchat/internal/services/providers'
import type { Library } from '@bindings/willchat/internal/services/library'
import LogoIcon from '@/assets/images/logo.svg'

const props = defineProps<{
  chatInput: string
  selectedModelKey: string
  selectedModelInfo: { providerId: string; modelId: string; modelName: string } | null
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
  isSnapMode?: boolean
}>()

const emit = defineEmits<{
  'update:chatInput': [value: string]
  'update:selectedModelKey': [value: string]
  'update:enableThinking': [value: boolean]
  'update:selectedLibraryIds': [value: number[]]
  send: []
  stop: []
  librarySelectionChange: []
  clearLibrarySelection: []
  loadLibraries: []
}>()

const { t } = useI18n()

const handleChatEnter = (event: KeyboardEvent) => {
  // Prevent sending when IME is composing (Chinese/Japanese/Korean input).
  // Some browsers report keyCode=229 during composition.

  const anyEvent = event as any
  if (anyEvent?.isComposing || anyEvent?.keyCode === 229) {
    return
  }

  event.preventDefault()
  emit('send')
}

const handleLibrarySelectionChange = () => {
  emit('librarySelectionChange')
}

const handleClearLibrarySelection = () => {
  emit('clearLibrarySelection')
}
</script>

<template>
  <div
    :class="
      cn(
        'flex px-6',
        chatMessages.length > 0 || isGenerating ? 'pb-4' : 'flex-1 items-center justify-center'
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
      <div v-if="chatMessages.length === 0 && !isGenerating" class="flex items-center gap-3">
        <LogoIcon class="size-10 text-foreground" />
        <div class="text-2xl font-semibold text-foreground">
          {{ t('app.title') }}
        </div>
      </div>

      <div
        class="w-full max-w-[800px] rounded-2xl border border-border bg-background px-4 pt-4 pb-3 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
      >
        <textarea
          :value="chatInput"
          :placeholder="t('assistant.placeholders.inputPlaceholder')"
          class="min-h-[64px] w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
          rows="2"
          @input="emit('update:chatInput', ($event.target as HTMLTextAreaElement).value)"
          @keydown.enter.exact="handleChatEnter"
        />

        <div class="mt-3 flex items-center justify-between">
          <div class="flex items-center gap-2">
            <TooltipProvider>
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
                          'h-8 w-auto rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40',
                          isSnapMode ? 'min-w-0 max-w-[160px]' : 'min-w-[160px] max-w-[240px]'
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
                      <SelectContent class="max-h-[260px]">
                        <SelectGroup>
                          <SelectLabel>{{ t('assistant.chat.selectModel') }}</SelectLabel>
                          <template v-for="pw in providersWithModels" :key="pw.provider.provider_id">
                            <SelectLabel class="mt-2 text-xs text-muted-foreground">
                              {{ pw.provider.name }}
                            </SelectLabel>
                            <template v-for="g in pw.model_groups" :key="g.type">
                              <template v-if="g.type === 'llm'">
                                <SelectItem
                                  v-for="m in g.models"
                                  :key="pw.provider.provider_id + '::' + m.model_id"
                                  :value="pw.provider.provider_id + '::' + m.model_id"
                                >
                                  {{ m.name }}
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

            <!-- Thinking mode toggle -->
            <TooltipProvider>
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

            <!-- Knowledge base multi-select using reka-ui Select with multiple -->
            <SelectRoot
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
                  <span class="inline-flex">
                    <Button
                      size="icon"
                      class="size-6 rounded-full bg-muted-foreground/20 text-muted-foreground disabled:opacity-100"
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
              @click="emit('send')"
            >
              <ArrowUp class="size-4" />
            </Button>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
