<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ArrowUp, Lightbulb } from 'lucide-vue-next'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { useNavigationStore } from '@/stores'
import { useAgents } from '@/pages/assistant/composables/useAgents'
import { useModelSelection } from '@/pages/assistant/composables/useModelSelection'
import LogoIcon from '@/assets/images/logo.svg'

const props = defineProps<{
  /** Currently selected library ID from the knowledge page */
  selectedLibraryId: number | null
  /** Tab ID of the parent knowledge page */
  tabId: string
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()

// Whether this tab is currently active
const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)

// Use composables for agent and model selection
const {
  agents,
  activeAgentId,
  loadAgents,
} = useAgents()

const {
  providersWithModels,
  selectedModelKey,
  hasModels,
  selectedModelInfo,
  loadModels,
  selectDefaultModel,
} = useModelSelection()

// Local state
const chatInput = ref('')
const enableThinking = ref(false)

// Computed: active agent
const activeAgent = computed(() => {
  if (activeAgentId.value == null) return null
  return agents.value.find((a) => a.id === activeAgentId.value) ?? null
})

// Can send: must have input, agent, and model
const canSend = computed(() => {
  return (
    !!activeAgentId.value &&
    chatInput.value.trim() !== '' &&
    !!selectedModelInfo.value
  )
})

// Reason why send is disabled
const sendDisabledReason = computed(() => {
  if (!activeAgentId.value) return t('assistant.placeholders.createAgentFirst')
  if (!selectedModelKey.value) return t('assistant.placeholders.selectModelFirst')
  if (!chatInput.value.trim()) return t('assistant.placeholders.enterToSend')
  return ''
})

const handleChatEnter = (event: KeyboardEvent) => {
  // Prevent sending when IME is composing
  const anyEvent = event as any
  if (anyEvent?.isComposing || anyEvent?.keyCode === 229) {
    return
  }
  event.preventDefault()
  handleSend()
}

const handleSend = () => {
  if (!canSend.value) return

  // Build library IDs array from the selected library
  const libraryIds = props.selectedLibraryId ? [props.selectedLibraryId] : []

  // Set pending chat data and open a new assistant tab
  navigationStore.setPendingChatAndOpenAssistant({
    chatInput: chatInput.value.trim(),
    libraryIds,
    selectedModelKey: selectedModelKey.value,
    agentId: activeAgentId.value ?? undefined,
    enableThinking: enableThinking.value,
  })

  // Clear input after sending
  chatInput.value = ''
}

// When agent changes, re-select default model
watch(activeAgentId, () => {
  selectDefaultModel(activeAgent.value, null)
})

// When models are loaded, select default model
watch(providersWithModels, () => {
  selectDefaultModel(activeAgent.value, null)
})

// Whether the currently selected model's provider is free (e.g. ChatWiki)
const selectedProviderIsFree = computed(() => {
  if (!selectedModelInfo.value?.providerId || !providersWithModels.value?.length) return false
  const pw = providersWithModels.value.find((p) => p.provider?.provider_id === selectedModelInfo.value?.providerId)
  return isProviderFree(pw)
})

function isProviderFree(pw: { provider?: { is_free?: boolean } } | undefined): boolean {
  if (!pw?.provider) return false
  return Boolean(pw.provider.is_free)
}

// When tab becomes active, refresh agents and models
// (user may have deleted agents or disabled models in other pages)
watch(isTabActive, (active) => {
  if (active) {
    void (async () => {
      await loadAgents()
      await loadModels()
    })()
  }
})

onMounted(async () => {
  await loadAgents()
  await loadModels()
  // Select default model based on the first agent
  selectDefaultModel(activeAgent.value, null)
})
</script>

<template>
  <div class="border-t border-border px-6 py-4">
    <div
      class="mx-auto w-full max-w-[800px] rounded-2xl border border-border bg-background px-4 pt-4 pb-3 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <textarea
        :value="chatInput"
        :placeholder="t('assistant.placeholders.inputPlaceholder')"
        class="min-h-[64px] w-full resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
        rows="2"
        @input="chatInput = ($event.target as HTMLTextAreaElement).value"
        @keydown.enter.exact="handleChatEnter"
      />

      <div class="mt-3 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <!-- Agent selector -->
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger as-child>
                <div class="min-w-0">
                  <Select
                    :model-value="activeAgentId != null ? String(activeAgentId) : undefined"
                    :disabled="agents.length === 0"
                    @update:model-value="(v: any) => v && (activeAgentId = Number(v))"
                  >
                    <SelectTrigger
                      class="h-8 w-auto min-w-[100px] max-w-[160px] rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40"
                    >
                      <div v-if="activeAgent" class="flex min-w-0 items-center gap-1.5">
                        <LogoIcon class="size-3.5 shrink-0 text-foreground" />
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

          <!-- Model selector -->
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger as-child>
                <div class="min-w-0">
                  <Select
                    :model-value="selectedModelKey"
                    :disabled="!hasModels"
                    @update:model-value="(v: any) => v && (selectedModelKey = String(v))"
                  >
                    <SelectTrigger
                      class="h-8 w-auto min-w-[160px] max-w-[240px] rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40"
                    >
                      <div v-if="selectedModelInfo" class="flex min-w-0 items-center gap-1.5">
                        <ProviderIcon
                          :icon="selectedModelInfo.providerId"
                          :size="14"
                          class="shrink-0 text-foreground"
                        />
                        <span class="truncate">{{ selectedModelInfo.modelName }}</span>
                        <span
                          v-if="selectedProviderIsFree"
                          class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                        >
                          {{ t('assistant.chat.freeBadge') }}
                        </span>
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
                  @click="enableThinking = !enableThinking"
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
        </div>

        <!-- Send button -->
        <TooltipProvider v-if="!canSend">
          <Tooltip>
            <TooltipTrigger as-child>
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
          @click="handleSend"
        >
          <ArrowUp class="size-4" />
        </Button>
      </div>
    </div>
  </div>
</template>
