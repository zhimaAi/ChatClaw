<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmojiPicker } from '@/components/ui/emoji-picker'
import { ProviderIcon } from '@/components/ui/provider-icon'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  OpenClawAgentsService,
  type OpenClawAgent,
} from '@bindings/chatclaw/internal/openclaw/agents'
import {
  ProvidersService,
  type ProviderWithModels,
} from '@bindings/chatclaw/internal/services/providers'

type TabKey = 'general' | 'advanced' | 'delete'

const props = defineProps<{
  open: boolean
  agent: OpenClawAgent | null
  initialTab?: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: [agent: OpenClawAgent]
  deleted: [id: number]
}>()

const { t } = useI18n()

const tab = ref<TabKey>('general')
const saving = ref(false)
const deleteConfirmOpen = ref(false)

const name = ref('')
const identityEmoji = ref('')
const identityTheme = ref('')

const providersWithModels = ref<ProviderWithModels[]>([])
const modelProviderId = ref('')
const modelId = ref('')
const modelName = ref('')
const modelChanged = ref(false)
const modelKey = ref('')

const TOOLS_PROFILE_DEFAULT = '__default__'
const TOOLS_PROFILES = [TOOLS_PROFILE_DEFAULT, 'minimal', 'coding', 'messaging', 'full'] as const
const SANDBOX_MODES = ['off', 'non-main', 'all'] as const
const HEARTBEAT_OFF = '__off__'
const HEARTBEAT_PRESETS = [HEARTBEAT_OFF, '5m', '15m', '30m', '1h', '6h'] as const

const sandboxMode = ref('off')
const groupChatMentionPatterns = ref('')
const toolsProfile = ref(TOOLS_PROFILE_DEFAULT)
const toolsAllowTags = ref<string[]>([])
const toolsAllowInput = ref('')
const toolsDenyTags = ref<string[]>([])
const toolsDenyInput = ref('')
const heartbeatEvery = ref(HEARTBEAT_OFF)
const paramsTemperature = ref('')
const paramsMaxTokens = ref('')

watch(
  () => props.open,
  (open) => {
    if (!open) return
    const validTabs: TabKey[] = ['general', 'advanced', 'delete']
    tab.value =
      props.initialTab && validTabs.includes(props.initialTab as TabKey)
        ? (props.initialTab as TabKey)
        : 'general'
    void loadModels()
  }
)

watch(
  () => props.agent,
  (agent) => {
    if (!agent) return
    name.value = agent.name ?? ''
    identityEmoji.value = agent.identity_emoji ?? ''
    identityTheme.value = agent.identity_theme ?? ''

    modelProviderId.value = agent.default_llm_provider_id ?? ''
    modelId.value = agent.default_llm_model_id ?? ''
    modelName.value = ''
    modelChanged.value = false
    modelKey.value =
      modelProviderId.value && modelId.value ? `${modelProviderId.value}::${modelId.value}` : ''

    sandboxMode.value = agent.sandbox_mode || 'off'

    const parseJsonArray = (v: string | undefined): string => {
      if (!v || v === '[]') return ''
      try {
        const arr = JSON.parse(v)
        return Array.isArray(arr) ? arr.join(', ') : ''
      } catch {
        return ''
      }
    }
    groupChatMentionPatterns.value = parseJsonArray(agent.group_chat_mention_patterns)
    toolsProfile.value = agent.tools_profile || TOOLS_PROFILE_DEFAULT
    toolsAllowTags.value = parseJsonArrayToList(agent.tools_allow)
    toolsAllowInput.value = ''
    toolsDenyTags.value = parseJsonArrayToList(agent.tools_deny)
    toolsDenyInput.value = ''
    heartbeatEvery.value = agent.heartbeat_every || HEARTBEAT_OFF
    paramsTemperature.value = agent.params_temperature ?? ''
    paramsMaxTokens.value = agent.params_max_tokens ?? ''
  },
  { immediate: true }
)

const hasDefaultModel = computed(() => modelProviderId.value !== '' && modelId.value !== '')
const isMainAgent = computed(() => props.agent?.openclaw_agent_id === 'main')

const selectedProviderIsFree = computed(() => {
  if (!modelProviderId.value || !providersWithModels.value.length) return false
  const pw = providersWithModels.value.find(
    (p) => p.provider?.provider_id === modelProviderId.value
  )
  return isProviderFree(pw)
})

function isProviderFree(pw: ProviderWithModels | undefined): boolean {
  if (!pw?.provider) return false
  const p = pw.provider as { is_free?: boolean }
  return Boolean(p.is_free)
}

const loadModels = async () => {
  try {
    const providers = await ProvidersService.ListProviders()
    const enabled = providers.filter((p) => p.enabled)
    const results: ProviderWithModels[] = []
    for (const p of enabled) {
      try {
        const withModels = await ProvidersService.GetProviderWithModels(p.provider_id)
        if (withModels) results.push(withModels)
      } catch (error: unknown) {
        console.warn(`Failed to load provider models (${p.provider_id}) in dialog:`, error)
      }
    }
    providersWithModels.value = results

    if (modelProviderId.value && modelId.value) {
      for (const pw of results) {
        if (pw.provider.provider_id !== modelProviderId.value) continue
        for (const group of pw.model_groups) {
          if (group.type !== 'llm') continue
          const m = group.models.find((x) => x.model_id === modelId.value)
          if (m) modelName.value = m.name
        }
      }
    }
  } catch (error: unknown) {
    console.warn('Failed to load models in dialog:', error)
  }
}

const onModelKeyChange = (val: any) => {
  if (typeof val !== 'string') return
  modelKey.value = val
  if (!val) {
    clearDefaultModel()
    return
  }
  const [p, m] = val.split('::')
  modelProviderId.value = p ?? ''
  modelId.value = m ?? ''
  modelName.value = ''
  for (const pw of providersWithModels.value) {
    if (pw.provider.provider_id !== modelProviderId.value) continue
    for (const group of pw.model_groups) {
      if (group.type !== 'llm') continue
      const found = group.models.find((x) => x.model_id === modelId.value)
      if (found) modelName.value = found.name
    }
  }
  modelChanged.value = true
}

const clearDefaultModel = () => {
  modelProviderId.value = ''
  modelId.value = ''
  modelName.value = ''
  modelChanged.value = true
  modelKey.value = ''
}

const isValid = computed(() => name.value.trim() !== '')

const handleClose = () => emit('update:open', false)

const toJsonArray = (csv: string): string => {
  const items = csv
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  return JSON.stringify(items)
}

const parseJsonArrayToList = (v: string | undefined): string[] => {
  if (!v || v === '[]') return []
  try {
    const arr = JSON.parse(v)
    return Array.isArray(arr) ? arr.filter((s: any) => typeof s === 'string' && s.trim()) : []
  } catch {
    return []
  }
}

const addToolsAllowTag = () => {
  const val = toolsAllowInput.value.trim()
  if (val && !toolsAllowTags.value.includes(val)) {
    toolsAllowTags.value.push(val)
  }
  toolsAllowInput.value = ''
}

const removeToolsAllowTag = (i: number) => {
  toolsAllowTags.value.splice(i, 1)
}

const addToolsDenyTag = () => {
  const val = toolsDenyInput.value.trim()
  if (val && !toolsDenyTags.value.includes(val)) {
    toolsDenyTags.value.push(val)
  }
  toolsDenyInput.value = ''
}

const removeToolsDenyTag = (i: number) => {
  toolsDenyTags.value.splice(i, 1)
}

const isHeartbeatCustom = computed(
  () =>
    heartbeatEvery.value !== HEARTBEAT_OFF &&
    !(HEARTBEAT_PRESETS as readonly string[]).includes(heartbeatEvery.value)
)

const heartbeatSelectValue = computed(() =>
  isHeartbeatCustom.value ? '__custom__' : heartbeatEvery.value
)

const customHeartbeat = ref('')

watch(
  () => heartbeatEvery.value,
  (v) => {
    if (isHeartbeatCustom.value) customHeartbeat.value = v
  },
  { immediate: true }
)

const onHeartbeatSelectChange = (val: any) => {
  if (val === '__custom__') {
    heartbeatEvery.value = customHeartbeat.value || '10m'
  } else {
    heartbeatEvery.value = val ?? HEARTBEAT_OFF
  }
}

const HEARTBEAT_PATTERN = /^\d+(ms|s|m|h)$/
const heartbeatError = computed(() => {
  if (!isHeartbeatCustom.value) return ''
  return HEARTBEAT_PATTERN.test(heartbeatEvery.value)
    ? ''
    : t('assistant.settings.advanced.heartbeatFormatError')
})

const onHeartbeatInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).value
  heartbeatEvery.value = raw.replace(/[^0-9a-z]/gi, '')
}

const clampTemperature = () => {
  if (paramsTemperature.value === '') return
  const n = parseFloat(paramsTemperature.value)
  if (isNaN(n)) {
    paramsTemperature.value = ''
    return
  }
  paramsTemperature.value = String(Math.round(Math.min(2, Math.max(0, n)) * 10) / 10)
}

const clampMaxTokens = () => {
  if (paramsMaxTokens.value === '') return
  const n = parseInt(paramsMaxTokens.value, 10)
  if (isNaN(n)) {
    paramsMaxTokens.value = ''
    return
  }
  paramsMaxTokens.value = String(Math.max(1, n))
}

const handleSave = async () => {
  if (!props.agent || !isValid.value || saving.value) return
  if (heartbeatError.value) {
    toast.error(heartbeatError.value)
    return
  }
  clampTemperature()
  clampMaxTokens()
  saving.value = true
  try {
    const wantsModelUpdate = modelChanged.value
    if (wantsModelUpdate && (modelProviderId.value === '') !== (modelId.value === '')) {
      throw new Error(t('assistant.errors.defaultModelIncomplete'))
    }
    const updated = await OpenClawAgentsService.UpdateAgent(props.agent.id, {
      name: name.value.trim(),
      icon: null,
      default_llm_provider_id: wantsModelUpdate ? modelProviderId.value : null,
      default_llm_model_id: wantsModelUpdate ? modelId.value : null,
      enable_llm_temperature: null,
      enable_llm_top_p: null,
      enable_llm_max_tokens: null,
      llm_temperature: null,
      llm_top_p: null,
      llm_max_context_count: null,
      llm_max_tokens: null,
      retrieval_match_threshold: null,
      retrieval_top_k: null,
      sandbox_mode: sandboxMode.value,
      sandbox_network: null,
      work_dir: null,
      mcp_enabled: null,
      mcp_server_ids: null,
      mcp_server_enabled_ids: null,
      identity_emoji: identityEmoji.value,
      identity_theme: identityTheme.value,
      group_chat_mention_patterns: toJsonArray(groupChatMentionPatterns.value),
      tools_profile: toolsProfile.value === TOOLS_PROFILE_DEFAULT ? '' : toolsProfile.value,
      tools_allow: JSON.stringify(toolsAllowTags.value),
      tools_deny: JSON.stringify(toolsDenyTags.value),
      heartbeat_every: heartbeatEvery.value === HEARTBEAT_OFF ? '' : heartbeatEvery.value,
      params_temperature: paramsTemperature.value,
      params_max_tokens: paramsMaxTokens.value,
    })
    if (!updated) {
      throw new Error(t('assistant.errors.updateFailed'))
    }
    emit('updated', updated)
    toast.success(t('assistant.toasts.updated'))
    modelChanged.value = false
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.updateFailed'))
  } finally {
    saving.value = false
  }
}

const handleDelete = async () => {
  if (!props.agent) return
  saving.value = true
  try {
    await OpenClawAgentsService.DeleteAgent(props.agent.id)
    emit('deleted', props.agent.id)
    toast.success(t('assistant.toasts.deleted'))
    emit('update:open', false)
  } catch (error: unknown) {
    toast.error(getErrorMessage(error) || t('assistant.errors.deleteFailed'))
  } finally {
    saving.value = false
    deleteConfirmOpen.value = false
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="xl" class="gap-0 p-0">
      <div class="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-3">
        <div class="text-base font-semibold text-foreground">
          {{ t('assistant.settings.title') }}
        </div>
      </div>

      <div class="h-[520px] overflow-hidden px-4 py-2">
        <div class="flex h-full">
          <div class="w-[140px] shrink-0 border-r border-border pr-4">
            <div class="flex flex-col gap-2">
              <button
                v-for="key in ['general', 'advanced', 'delete'] as const"
                :key="key"
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === key
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = key"
              >
                {{ t(`assistant.settings.tabs.${key}`) }}
              </button>
            </div>
          </div>

          <div class="min-w-0 flex-1 pl-4">
            <div
              class="h-full overflow-y-auto overflow-x-hidden rounded-2xl border border-border bg-card p-6 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <!-- General: identity + model -->
              <div v-if="tab === 'general'" class="flex flex-col gap-5">
                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.fields.name') }}
                    <span class="text-destructive">*</span>
                  </label>
                  <Input
                    v-model="name"
                    :placeholder="t('assistant.fields.namePlaceholder')"
                    maxlength="100"
                  />
                </div>

                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.fields.identityEmoji') }}
                  </label>
                  <EmojiPicker v-model="identityEmoji" />
                </div>

                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.fields.identityTheme') }}
                  </label>
                  <Input
                    v-model="identityTheme"
                    :placeholder="t('assistant.fields.identityThemePlaceholder')"
                    maxlength="200"
                  />
                </div>

                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.model.defaultModel') }}
                  </label>
                  <div class="flex min-w-0 items-center gap-2">
                    <Select :model-value="modelKey" @update:model-value="onModelKeyChange">
                      <SelectTrigger
                        class="h-9 w-full rounded-md border border-border bg-background"
                      >
                        <div v-if="hasDefaultModel" class="flex min-w-0 items-center gap-2">
                          <ProviderIcon
                            :icon="modelProviderId"
                            :size="16"
                            class="text-foreground"
                          />
                          <div class="min-w-0 truncate text-sm font-medium text-foreground">
                            {{ modelName || modelId }}
                          </div>
                          <span
                            v-if="selectedProviderIsFree"
                            class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                          >
                            {{ t('assistant.chat.freeBadge') }}
                          </span>
                        </div>
                        <div v-else class="text-sm text-muted-foreground">
                          {{ t('assistant.settings.model.noDefaultModel') }}
                        </div>
                      </SelectTrigger>
                      <SelectContent class="max-h-[260px]">
                        <SelectGroup>
                          <SelectLabel>{{
                            t('assistant.settings.model.defaultModel')
                          }}</SelectLabel>
                          <template
                            v-for="pw in providersWithModels"
                            :key="pw.provider.provider_id"
                          >
                            <SelectLabel class="mt-2 flex items-center gap-1.5">
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
                    <Button
                      v-if="hasDefaultModel"
                      size="icon"
                      variant="ghost"
                      :disabled="saving"
                      :title="t('assistant.settings.model.clear')"
                      @click="clearDefaultModel"
                    >
                      <Trash2 class="size-4" />
                    </Button>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.model.defaultModelHint') }}
                  </p>
                </div>
              </div>

              <!-- Advanced -->
              <div v-else-if="tab === 'advanced'" class="flex flex-col gap-5">
                <!-- Sandbox mode -->
                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.advanced.sandboxMode') }}
                  </label>
                  <Select v-model="sandboxMode">
                    <SelectTrigger class="h-9 w-full">
                      <span class="text-sm">{{
                        t(`assistant.settings.advanced.sandbox_${sandboxMode}`)
                      }}</span>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem v-for="mode in SANDBOX_MODES" :key="mode" :value="mode">
                        {{ t(`assistant.settings.advanced.sandbox_${mode}`) }}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.advanced.sandboxModeHint') }}
                  </p>
                </div>

                <!-- Tools -->
                <div class="flex flex-col gap-3">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.advanced.tools') }}
                  </div>
                  <div class="flex flex-col gap-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ t('assistant.settings.advanced.toolsProfile') }}
                    </label>
                    <Select v-model="toolsProfile">
                      <SelectTrigger class="h-9 w-full">
                        <span class="text-sm">
                          {{
                            toolsProfile === TOOLS_PROFILE_DEFAULT
                              ? t('assistant.settings.advanced.toolsProfile_default')
                              : t(`assistant.settings.advanced.toolsProfile_${toolsProfile}`)
                          }}
                        </span>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem v-for="p in TOOLS_PROFILES" :key="p" :value="p">
                          {{
                            p === TOOLS_PROFILE_DEFAULT
                              ? t('assistant.settings.advanced.toolsProfile_default')
                              : t(`assistant.settings.advanced.toolsProfile_${p}`)
                          }}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div class="flex flex-col gap-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ t('assistant.settings.advanced.toolsAllow') }}
                    </label>
                    <div
                      class="flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1.5 min-h-9"
                    >
                      <span
                        v-for="(tag, i) in toolsAllowTags"
                        :key="i"
                        class="inline-flex items-center gap-1 rounded bg-muted px-2 py-0.5 text-xs text-foreground"
                      >
                        {{ tag }}
                        <button
                          class="text-muted-foreground hover:text-foreground"
                          @click="removeToolsAllowTag(i)"
                        >
                          &times;
                        </button>
                      </span>
                      <input
                        v-model="toolsAllowInput"
                        class="min-w-[80px] flex-1 border-0 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                        :placeholder="
                          toolsAllowTags.length
                            ? ''
                            : t('assistant.settings.advanced.toolsAllowPlaceholder')
                        "
                        @keydown.enter.prevent="addToolsAllowTag"
                        @keydown.,.prevent="addToolsAllowTag"
                        @blur="addToolsAllowTag"
                      />
                    </div>
                  </div>
                  <div class="flex flex-col gap-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ t('assistant.settings.advanced.toolsDeny') }}
                    </label>
                    <div
                      class="flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1.5 min-h-9"
                    >
                      <span
                        v-for="(tag, i) in toolsDenyTags"
                        :key="i"
                        class="inline-flex items-center gap-1 rounded bg-muted px-2 py-0.5 text-xs text-foreground"
                      >
                        {{ tag }}
                        <button
                          class="text-muted-foreground hover:text-foreground"
                          @click="removeToolsDenyTag(i)"
                        >
                          &times;
                        </button>
                      </span>
                      <input
                        v-model="toolsDenyInput"
                        class="min-w-[80px] flex-1 border-0 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                        :placeholder="
                          toolsDenyTags.length
                            ? ''
                            : t('assistant.settings.advanced.toolsDenyPlaceholder')
                        "
                        @keydown.enter.prevent="addToolsDenyTag"
                        @keydown.,.prevent="addToolsDenyTag"
                        @blur="addToolsDenyTag"
                      />
                    </div>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.advanced.toolsHint') }}
                  </p>
                </div>

                <!-- Group chat -->
                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.advanced.groupChatMentionPatterns') }}
                  </label>
                  <div class="flex items-center gap-2">
                    <Input
                      v-model="groupChatMentionPatterns"
                      class="flex-1"
                      :placeholder="
                        t('assistant.settings.advanced.groupChatMentionPatternsPlaceholder')
                      "
                    />
                    <Button
                      v-if="!groupChatMentionPatterns"
                      variant="outline"
                      size="sm"
                      class="shrink-0 text-xs"
                      @click="groupChatMentionPatterns = '@' + (name || 'assistant')"
                    >
                      {{ t('assistant.settings.advanced.groupChatInsertPreset') }}
                    </Button>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.advanced.groupChatMentionPatternsHint') }}
                  </p>
                </div>

                <!-- Heartbeat -->
                <div class="flex flex-col gap-1.5">
                  <label class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.advanced.heartbeat') }}
                  </label>
                  <Select
                    :model-value="heartbeatSelectValue"
                    @update:model-value="onHeartbeatSelectChange"
                  >
                    <SelectTrigger class="h-9 w-full">
                      <span class="text-sm">
                        {{
                          heartbeatEvery === HEARTBEAT_OFF
                            ? t('assistant.settings.advanced.heartbeat_off')
                            : isHeartbeatCustom
                              ? t('assistant.settings.advanced.heartbeat_custom')
                              : heartbeatEvery
                        }}
                      </span>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem v-for="p in HEARTBEAT_PRESETS" :key="p" :value="p">
                        {{
                          p === HEARTBEAT_OFF ? t('assistant.settings.advanced.heartbeat_off') : p
                        }}
                      </SelectItem>
                      <SelectItem value="__custom__">{{
                        t('assistant.settings.advanced.heartbeat_custom')
                      }}</SelectItem>
                    </SelectContent>
                  </Select>
                  <div v-if="isHeartbeatCustom" class="flex flex-col gap-1">
                    <Input
                      :model-value="heartbeatEvery"
                      class="h-9"
                      :class="heartbeatError ? 'border-destructive' : ''"
                      :placeholder="t('assistant.settings.advanced.heartbeatPlaceholder')"
                      @input="onHeartbeatInput"
                    />
                    <p v-if="heartbeatError" class="text-xs text-destructive">
                      {{ heartbeatError }}
                    </p>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.advanced.heartbeatHint') }}
                  </p>
                </div>

                <!-- Model params -->
                <div class="flex flex-col gap-3">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.advanced.params') }}
                  </div>
                  <div class="flex flex-col gap-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ t('assistant.settings.advanced.paramsTemperature') }}
                    </label>
                    <Input
                      v-model="paramsTemperature"
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      :placeholder="t('assistant.settings.advanced.paramsTemperaturePlaceholder')"
                      @blur="clampTemperature"
                    />
                  </div>
                  <div class="flex flex-col gap-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ t('assistant.settings.advanced.paramsMaxTokens') }}
                    </label>
                    <Input
                      v-model="paramsMaxTokens"
                      type="number"
                      step="1"
                      min="1"
                      :placeholder="t('assistant.settings.advanced.paramsMaxTokensPlaceholder')"
                      @blur="clampMaxTokens"
                    />
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.advanced.paramsHint') }}
                  </p>
                </div>
              </div>

              <!-- Delete agent -->
              <div v-else class="flex h-full flex-col items-center justify-center gap-4">
                <div class="text-base font-semibold text-foreground">
                  {{ t('assistant.settings.delete.title') }}
                </div>
                <div class="max-w-[420px] text-center text-sm text-muted-foreground">
                  {{
                    isMainAgent
                      ? t('assistant.settings.delete.protected')
                      : t('assistant.settings.delete.hint')
                  }}
                </div>

                <Button
                  variant="outline"
                  class="border-border text-foreground hover:bg-accent"
                  :disabled="saving || isMainAgent"
                  @click="deleteConfirmOpen = true"
                >
                  {{ t('assistant.settings.delete.action') }}
                </Button>

                <AlertDialog
                  :open="deleteConfirmOpen"
                  @update:open="(v) => (deleteConfirmOpen = v)"
                >
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>{{
                        t('assistant.settings.delete.confirmTitle')
                      }}</AlertDialogTitle>
                      <AlertDialogDescription>
                        {{
                          t('assistant.settings.delete.confirmDesc', {
                            name: props.agent?.name ?? '',
                          })
                        }}
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel :disabled="saving">
                        {{ t('assistant.actions.cancel') }}
                      </AlertDialogCancel>
                      <AlertDialogAction
                        class="bg-foreground text-background hover:bg-foreground/90"
                        :disabled="saving"
                        @click="handleDelete"
                      >
                        {{ t('assistant.settings.delete.action') }}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div
        class="flex items-center justify-end gap-2 border-t border-border bg-background px-4 py-3"
      >
        <Button variant="outline" :disabled="saving" @click="handleClose">
          {{ t('assistant.actions.cancel') }}
        </Button>
        <Button v-if="tab !== 'delete'" :disabled="!isValid || saving" @click="handleSave">
          {{ t('assistant.actions.save') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
