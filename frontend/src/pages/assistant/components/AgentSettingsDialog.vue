<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import {
  Trash2,
  ShieldCheck,
  Monitor,
  Globe,
  FolderOpen,
  RotateCcw,
  AlertTriangle,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useThemeLogo } from '@/composables/useLogo'
import { Dialogs } from '@wailsio/runtime'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { defaultAvatars } from '@/assets/avatars'
import SliderWithTicks from './SliderWithTicks.vue'
import SliderWithMarks from '@/pages/knowledge/components/SliderWithMarks.vue'
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
import { AgentsService, type Agent } from '@bindings/chatclaw/internal/services/agents'
import { Switch } from '@/components/ui/switch'
import {
  ProvidersService,
  type ProviderWithModels,
} from '@bindings/chatclaw/internal/services/providers'
import * as ToolchainService from '@bindings/chatclaw/internal/services/toolchain/toolchainservice'

type TabKey = 'model' | 'prompt' | 'workspace' | 'retrieval' | 'delete'

const props = defineProps<{
  open: boolean
  agent: Agent | null
  initialTab?: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: [agent: Agent]
  deleted: [id: number]
}>()

const { t } = useI18n()
const { logoSrc } = useThemeLogo()

const tab = ref<TabKey>('model')
const saving = ref(false)
const deleteConfirmOpen = ref(false)

// prompt tab fields
const name = ref('')
const prompt = ref('')
const icon = ref<string>('') // data URL
const iconChanged = ref(false)

// model tab fields
const temperature = ref(0.5)
const topP = ref(1.0)
const contextCount = ref(50)
const maxTokens = ref(1000)
const retrievalMatchThreshold = ref(0.5)
const retrievalTopK = ref<number[]>([20])

const enableTemperature = ref(false)
const enableTopP = ref(false)
const enableMaxTokens = ref(false)

// workspace tab fields
const sandboxMode = ref('codex')
const sandboxNetwork = ref(true)
const workDir = ref('')
const defaultWorkDir = ref('')
const codexInstalled = ref(false)

const providersWithModels = ref<ProviderWithModels[]>([])
const modelProviderId = ref('')
const modelId = ref('')
const modelName = ref('')
const modelChanged = ref(false)
const modelKey = ref('')

watch(
  () => props.open,
  (open) => {
    if (!open) return
    const validTabs: TabKey[] = ['model', 'prompt', 'workspace', 'retrieval', 'delete']
    tab.value =
      props.initialTab && validTabs.includes(props.initialTab as TabKey)
        ? (props.initialTab as TabKey)
        : 'model'
    void loadModels()
    void AgentsService.GetDefaultWorkDir().then((dir) => {
      defaultWorkDir.value = dir
    })
    void ToolchainService.GetToolStatus('codex').then((status) => {
      codexInstalled.value = status?.installed ?? false
    })
  }
)

let unsubscribeToolchain: (() => void) | null = null

onMounted(() => {
  unsubscribeToolchain = Events.On('toolchain:status', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.name === 'codex') {
      codexInstalled.value = !!data.installed
    }
  })
})

onUnmounted(() => {
  unsubscribeToolchain?.()
  unsubscribeToolchain = null
})

watch(
  () => props.agent,
  (agent) => {
    if (!agent) return
    name.value = agent.name ?? ''
    prompt.value = agent.prompt ?? ''
    icon.value = agent.icon ?? ''
    iconChanged.value = false

    temperature.value = agent.llm_temperature ?? 0.5
    topP.value = agent.llm_top_p ?? 1.0
    contextCount.value = agent.llm_max_context_count ?? 50
    maxTokens.value = agent.llm_max_tokens ?? 1000
    retrievalMatchThreshold.value = agent.retrieval_match_threshold ?? 0.5
    retrievalTopK.value = [agent.retrieval_top_k ?? 20]

    enableTemperature.value = agent.enable_llm_temperature ?? false
    enableTopP.value = agent.enable_llm_top_p ?? false
    enableMaxTokens.value = agent.enable_llm_max_tokens ?? false

    modelProviderId.value = agent.default_llm_provider_id ?? ''
    modelId.value = agent.default_llm_model_id ?? ''
    modelName.value = ''
    modelChanged.value = false
    modelKey.value =
      modelProviderId.value && modelId.value ? `${modelProviderId.value}::${modelId.value}` : ''

    sandboxMode.value = agent.sandbox_mode || 'codex'
    sandboxNetwork.value = agent.sandbox_network ?? true
    workDir.value = agent.work_dir ?? ''
  },
  { immediate: true }
)

const hasDefaultModel = computed(() => modelProviderId.value !== '' && modelId.value !== '')

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

const displayContextCount = computed(() => {
  return contextCount.value >= 200
    ? t('assistant.settings.model.unlimited')
    : String(contextCount.value)
})

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
        // Keep dialog usable even if one provider is down.
        console.warn(`Failed to load provider models (${p.provider_id}) in dialog:`, error)
      }
    }
    providersWithModels.value = results

    // resolve current model name
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
    // 弹窗内加载失败不阻塞用户操作，仅记录警告
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

const handleSelectDefaultAvatar = (src: string) => {
  icon.value = src
  iconChanged.value = true
}

const handlePickIcon = async () => {
  if (saving.value) return
  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('assistant.icon.pickTitle'),
      Filters: [
        {
          DisplayName: t('assistant.icon.filterImages'),
          Pattern: '*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg',
        },
      ],
    })
    if (!path) return
    icon.value = await AgentsService.ReadIconFile(path)
    iconChanged.value = true
  } catch (error) {
    // User cancelled the file dialog — not an error
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to pick icon:', error)
  }
}

const isWindows = navigator.platform.toLowerCase().includes('win')
const pathSep = isWindows ? '\\' : '/'

const workDirHint = computed(() => {
  const base = workDir.value || defaultWorkDir.value
  if (!base) return ''
  return t('assistant.settings.workspace.workDirHint', { basePath: base, sep: pathSep })
})

const handleSelectWorkDir = async () => {
  try {
    const result = await Dialogs.OpenFile({
      Title: t('assistant.settings.workspace.selectDir'),
      CanChooseFiles: false,
      CanChooseDirectories: true,
      AllowsMultipleSelection: false,
    })
    if (result && typeof result === 'string') {
      workDir.value = result
    } else if (Array.isArray(result) && result.length > 0) {
      workDir.value = result[0]
    }
  } catch (error) {
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to select directory:', error)
  }
}

const handleSave = async () => {
  if (!props.agent || !isValid.value || saving.value) return
  saving.value = true
  try {
    const wantsModelUpdate = modelChanged.value
    if (wantsModelUpdate && (modelProviderId.value === '') !== (modelId.value === '')) {
      throw new Error(t('assistant.errors.defaultModelIncomplete'))
    }
    const updated = await AgentsService.UpdateAgent(props.agent.id, {
      name: name.value.trim(),
      prompt: prompt.value.trim(),
      icon: iconChanged.value ? icon.value : null,
      default_llm_provider_id: wantsModelUpdate ? modelProviderId.value : null,
      default_llm_model_id: wantsModelUpdate ? modelId.value : null,
      enable_llm_temperature: enableTemperature.value,
      enable_llm_top_p: enableTopP.value,
      enable_llm_max_tokens: enableMaxTokens.value,
      llm_temperature: temperature.value,
      llm_top_p: topP.value,
      llm_max_context_count: contextCount.value,
      llm_max_tokens: maxTokens.value,
      retrieval_match_threshold: retrievalMatchThreshold.value,
      retrieval_top_k: retrievalTopK.value[0] ?? 20,
      sandbox_mode: sandboxMode.value,
      sandbox_network: sandboxNetwork.value,
      work_dir: workDir.value,
      mcp_enabled: null,
      mcp_server_ids: null,
      mcp_server_enabled_ids: null,
    })
    if (!updated) {
      throw new Error(t('assistant.errors.updateFailed'))
    }
    emit('updated', updated)
    toast.success(t('assistant.toasts.updated'))
    emit('update:open', false)
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
    await AgentsService.DeleteAgent(props.agent.id)
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
      <!-- 头部：标题（关闭按钮由 DialogContent 自带） -->
      <div class="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-3">
        <div class="text-base font-semibold text-foreground">
          {{ t('assistant.settings.title') }}
        </div>
      </div>

      <!-- 内容区：固定高度，内部不随 tab 抖动 -->
      <div class="h-[480px] overflow-hidden px-4 py-2">
        <div class="flex h-full">
          <!-- 左侧 tabs（独立区域） -->
          <div class="w-[140px] shrink-0 border-r border-border pr-4">
            <div class="flex flex-col gap-2">
              <button
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === 'model'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = 'model'"
              >
                {{ t('assistant.settings.tabs.model') }}
              </button>
              <button
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === 'prompt'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = 'prompt'"
              >
                {{ t('assistant.settings.tabs.prompt') }}
              </button>
              <button
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === 'workspace'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = 'workspace'"
              >
                {{ t('assistant.settings.tabs.workspace') }}
              </button>
              <button
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === 'retrieval'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = 'retrieval'"
              >
                {{ t('assistant.settings.tabs.retrieval') }}
              </button>
              <button
                :class="
                  cn(
                    'w-full rounded-md px-3 py-2 text-left text-sm font-medium transition-colors',
                    tab === 'delete'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )
                "
                @click="tab = 'delete'"
              >
                {{ t('assistant.settings.tabs.delete') }}
              </button>
            </div>
          </div>

          <!-- 右侧卡片（明显边框，固定高度，不随 tab 抖动） -->
          <div class="min-w-0 flex-1 pl-4">
            <div
              class="h-full overflow-y-auto overflow-x-hidden rounded-2xl border border-border bg-card p-6 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
            >
              <!-- 模型设置 -->
              <div v-if="tab === 'model'" class="flex flex-col gap-5">
                <div class="flex items-center justify-between gap-4">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.model.defaultModel') }}
                  </div>
                  <div class="flex min-w-0 items-center gap-2">
                    <Select :model-value="modelKey" @update:model-value="onModelKeyChange">
                      <SelectTrigger
                        class="h-9 w-[240px] rounded-md border border-border bg-background"
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
                      size="icon"
                      variant="ghost"
                      :disabled="saving || !hasDefaultModel"
                      :title="t('assistant.settings.model.clear')"
                      @click="clearDefaultModel"
                    >
                      <Trash2 class="size-4" />
                    </Button>
                  </div>
                </div>

                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between gap-3">
                    <div class="min-w-0">
                      <div class="text-sm font-medium text-foreground">
                        {{ t('assistant.settings.model.temperature') }}
                      </div>
                      <div class="text-xs text-muted-foreground">
                        {{ t('assistant.settings.model.temperatureHint') }}
                      </div>
                    </div>
                    <Switch v-model="enableTemperature" />
                  </div>
                  <SliderWithTicks
                    v-if="enableTemperature"
                    v-model="temperature"
                    :min="0"
                    :max="2"
                    :step="0.05"
                    :ticks="[
                      { value: 0, label: '0' },
                      { value: 0.5, label: '0.5' },
                      { value: 1, label: '1' },
                      { value: 1.5, label: '1.5' },
                      { value: 2, label: '2' },
                    ]"
                    :format-value="(v) => v.toFixed(2)"
                  />
                </div>

                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between gap-3">
                    <div class="min-w-0">
                      <div class="text-sm font-medium text-foreground">
                        {{ t('assistant.settings.model.topP') }}
                      </div>
                      <div class="text-xs text-muted-foreground">
                        {{ t('assistant.settings.model.topPHint') }}
                      </div>
                    </div>
                    <Switch v-model="enableTopP" />
                  </div>
                  <SliderWithTicks
                    v-if="enableTopP"
                    v-model="topP"
                    :min="0"
                    :max="1"
                    :step="0.01"
                    :ticks="[
                      { value: 0, label: '0' },
                      { value: 0.25, label: '0.25' },
                      { value: 0.5, label: '0.5' },
                      { value: 0.75, label: '0.75' },
                      { value: 1, label: '1' },
                    ]"
                    :format-value="(v) => v.toFixed(2)"
                  />
                </div>

                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between">
                    <div class="text-sm font-medium text-foreground">
                      {{ t('assistant.settings.model.contextCount') }}
                    </div>
                    <div class="text-sm text-muted-foreground">
                      {{ displayContextCount }}
                    </div>
                  </div>
                  <SliderWithTicks
                    v-model="contextCount"
                    :min="0"
                    :max="200"
                    :step="1"
                    :ticks="[
                      { value: 0, label: '0' },
                      { value: 50, label: '50' },
                      { value: 100, label: '100' },
                      { value: 150, label: '150' },
                      { value: 200, label: t('assistant.settings.model.unlimited') },
                    ]"
                    :format-value="() => ''"
                  />
                </div>

                <div class="flex items-center justify-between gap-4">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.model.maxTokens') }}
                  </div>
                  <div class="flex items-center gap-2">
                    <Input
                      v-if="enableMaxTokens"
                      v-model.number="maxTokens"
                      type="number"
                      min="1"
                      max="200000"
                      class="h-9 w-[160px]"
                    />
                    <Switch v-model="enableMaxTokens" />
                  </div>
                </div>
              </div>

              <!-- 提示词设置 -->
              <div v-else-if="tab === 'prompt'" class="flex flex-col gap-4">
                <div class="flex flex-col items-center gap-2">
                  <button
                    class="flex size-icon-box items-center justify-center rounded-icon-box border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
                    type="button"
                    @click="handlePickIcon"
                  >
                    <img v-if="icon" :src="icon" class="size-icon-lg rounded-md object-contain" />
                    <img v-else :src="logoSrc" class="size-icon-lg" alt="ChatClaw logo" />
                  </button>
                  <div class="text-xs text-muted-foreground">
                    {{ t('assistant.icon.hint') }}
                  </div>
                </div>

                <div class="flex flex-col gap-2">
                  <div class="text-xs text-muted-foreground">
                    {{ t('assistant.icon.defaultAvatars') }}
                  </div>
                  <div class="flex flex-wrap gap-3">
                    <button
                      v-for="avatar in defaultAvatars"
                      :key="avatar.id"
                      type="button"
                      class="relative flex size-12 items-center justify-center rounded-xl border transition-colors"
                      :class="
                        icon === avatar.src
                          ? 'border-primary bg-primary/10'
                          : 'border-border bg-background hover:border-foreground/40 hover:bg-muted/60 dark:border-white/10'
                      "
                      @click="handleSelectDefaultAvatar(avatar.src)"
                    >
                      <img :src="avatar.src" class="size-10 rounded-lg object-cover" />
                      <div
                        v-if="icon === avatar.src"
                        class="absolute inset-0 flex items-center justify-center rounded-xl bg-black/40"
                      >
                        <span
                          class="flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm"
                        >
                          ✓
                        </span>
                      </div>
                    </button>
                  </div>
                </div>

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
                    {{ t('assistant.fields.prompt') }}
                  </label>
                  <textarea
                    v-model="prompt"
                    :placeholder="t('assistant.fields.promptPlaceholder')"
                    maxlength="1000"
                    class="min-h-[200px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </div>
              </div>

              <!-- 工作区设置 -->
              <div v-else-if="tab === 'workspace'" class="flex min-w-0 flex-col gap-4">
                <div class="flex flex-col gap-3">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.workspace.sandboxMode') }}
                  </div>
                  <div class="flex gap-2">
                    <button
                      class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-colors"
                      :class="
                        sandboxMode === 'codex'
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'
                      "
                      @click="sandboxMode = 'codex'"
                    >
                      <ShieldCheck class="size-4" />
                      {{ t('assistant.settings.workspace.modeCodex') }}
                    </button>
                    <button
                      class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-colors"
                      :class="
                        sandboxMode === 'native'
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'
                      "
                      @click="sandboxMode = 'native'"
                    >
                      <Monitor class="size-4" />
                      {{ t('assistant.settings.workspace.modeNative') }}
                    </button>
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{
                      sandboxMode === 'codex'
                        ? t('assistant.settings.workspace.codexDesc')
                        : t('assistant.settings.workspace.nativeDesc')
                    }}
                  </p>
                  <div
                    v-if="sandboxMode === 'codex' && !codexInstalled"
                    class="flex items-start gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2.5 dark:border-white/10 dark:bg-white/5"
                  >
                    <AlertTriangle class="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
                    <p class="text-xs text-muted-foreground">
                      {{ t('assistant.settings.workspace.codexNotInstalled') }}
                    </p>
                  </div>
                </div>

                <div v-if="sandboxMode === 'codex'" class="flex flex-col gap-2">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <Globe class="size-4 text-muted-foreground" />
                      <span class="text-sm font-medium text-foreground">
                        {{ t('assistant.settings.workspace.networkAccess') }}
                      </span>
                    </div>
                    <Switch v-model="sandboxNetwork" />
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.workspace.networkAccessDesc') }}
                  </p>
                </div>

                <div class="flex flex-col gap-2">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.workspace.workDir') }}
                  </div>
                  <p class="text-xs text-muted-foreground">
                    {{ t('assistant.settings.workspace.workDirDesc') }}
                  </p>
                  <div class="flex min-w-0 items-center gap-2">
                    <span
                      class="min-w-0 flex-1 truncate rounded-md border border-border bg-background px-3 py-2 text-sm text-muted-foreground"
                      :title="workDir || defaultWorkDir"
                    >
                      {{ workDir || defaultWorkDir }}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      class="shrink-0 gap-1.5"
                      @click="handleSelectWorkDir"
                    >
                      <FolderOpen class="size-3.5 shrink-0" />
                      {{ t('assistant.settings.workspace.changeDir') }}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="-ml-1 -mr-1.5 shrink-0 text-muted-foreground"
                      :title="t('assistant.settings.workspace.resetDir')"
                      @click="workDir = defaultWorkDir"
                    >
                      <RotateCcw class="size-3.5" />
                    </Button>
                  </div>
                  <p class="overflow-hidden break-all text-xs font-mono text-muted-foreground/70">
                    {{ workDirHint }}
                  </p>
                </div>
              </div>

              <!-- 知识库检索设置 -->
              <div v-else-if="tab === 'retrieval'" class="flex flex-col gap-5">
                <div class="flex items-center justify-between gap-4">
                  <div class="text-sm font-medium text-foreground">
                    {{ t('assistant.settings.retrieval.matchThreshold') }}
                  </div>
                  <Input
                    v-model.number="retrievalMatchThreshold"
                    type="number"
                    min="0"
                    max="1"
                    step="0.01"
                    class="h-9 w-[160px]"
                  />
                </div>

                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between">
                    <div class="text-sm font-medium text-foreground">
                      {{ t('assistant.settings.retrieval.topK') }}
                    </div>
                    <div class="text-sm text-muted-foreground tabular-nums">
                      {{ retrievalTopK[0] ?? 20 }}
                    </div>
                  </div>
                  <SliderWithMarks
                    v-model="retrievalTopK"
                    :min="1"
                    :max="50"
                    :step="1"
                    :disabled="saving"
                    :marks="[
                      { value: 1, label: '1' },
                      {
                        value: 20,
                        label: t('assistant.settings.retrieval.default'),
                        emphasize: true,
                      },
                      { value: 30, label: '30' },
                      { value: 50, label: '50' },
                    ]"
                  />
                </div>
              </div>

              <!-- 删除助手 -->
              <div v-else class="flex h-full flex-col items-center justify-center gap-4">
                <div class="text-base font-semibold text-foreground">
                  {{ t('assistant.settings.delete.title') }}
                </div>
                <div class="max-w-[420px] text-center text-sm text-muted-foreground">
                  {{ t('assistant.settings.delete.hint') }}
                </div>

                <Button
                  variant="outline"
                  class="border-border text-foreground hover:bg-accent"
                  :disabled="saving"
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

      <!-- 底部：操作按钮 -->
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
