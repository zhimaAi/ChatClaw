<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ProviderIcon } from '@/components/ui/provider-icon'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ProvidersService,
  type ProviderWithModels,
} from '@bindings/chatclaw/internal/services/providers'

export interface CreateAgentData {
  name: string
  icon: string
  defaultLLMProviderId: string
  defaultLLMModelId: string
  identityEmoji: string
  identityTheme: string
  groupChatMentionPatterns: string
  toolsProfile: string
  toolsAllow: string
  toolsDeny: string
  heartbeatEvery: string
  paramsTemperature: string
  paramsMaxTokens: string
}

const props = defineProps<{
  open: boolean
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  create: [data: CreateAgentData]
}>()

const { t } = useI18n()

const name = ref('')
const identityEmoji = ref('')
const identityTheme = ref('')

const providersWithModels = ref<ProviderWithModels[]>([])
const modelProviderId = ref('')
const modelId = ref('')
const modelName = ref('')
const modelKey = ref('')

const isValid = computed(() => name.value.trim() !== '')
const hasDefaultModel = computed(() => modelProviderId.value !== '' && modelId.value !== '')

function isProviderFree(pw: ProviderWithModels | undefined): boolean {
  if (!pw?.provider) return false
  const p = pw.provider as { is_free?: boolean }
  return Boolean(p.is_free)
}

const selectedProviderIsFree = computed(() => {
  if (!modelProviderId.value || !providersWithModels.value.length) return false
  const pw = providersWithModels.value.find(
    (p) => p.provider?.provider_id === modelProviderId.value
  )
  return isProviderFree(pw)
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
        console.warn(`Failed to load provider models (${p.provider_id}) in create dialog:`, error)
      }
    }
    providersWithModels.value = results
  } catch (error: unknown) {
    console.warn('Failed to load models in create dialog:', error)
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    name.value = ''
    identityEmoji.value = ''
    identityTheme.value = ''
    modelProviderId.value = ''
    modelId.value = ''
    modelName.value = ''
    modelKey.value = ''
    void loadModels()
  }
)

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
}

const clearDefaultModel = () => {
  modelProviderId.value = ''
  modelId.value = ''
  modelName.value = ''
  modelKey.value = ''
}

const handleClose = () => emit('update:open', false)

const handleCreate = () => {
  if (!isValid.value || props.loading) return
  emit('create', {
    name: name.value.trim(),
    icon: '',
    defaultLLMProviderId: modelProviderId.value,
    defaultLLMModelId: modelId.value,
    identityEmoji: identityEmoji.value,
    identityTheme: identityTheme.value,
    groupChatMentionPatterns: '[]',
    toolsProfile: '',
    toolsAllow: '[]',
    toolsDeny: '[]',
    heartbeatEvery: '',
    paramsTemperature: '',
    paramsMaxTokens: '',
  })
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('assistant.create.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
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

        <div class="grid grid-cols-2 gap-3">
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-foreground">
              {{ t('assistant.fields.identityEmoji') }}
            </label>
            <Input
              v-model="identityEmoji"
              :placeholder="t('assistant.fields.identityEmojiPlaceholder')"
              maxlength="10"
            />
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
        </div>

        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('assistant.settings.model.defaultModel') }}
          </label>
          <div class="flex min-w-0 items-center gap-2">
            <Select :model-value="modelKey" @update:model-value="onModelKeyChange">
              <SelectTrigger class="h-9 w-full rounded-md border border-border bg-background">
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
                  <SelectLabel>{{ t('assistant.settings.model.defaultModel') }}</SelectLabel>
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
              :disabled="loading"
              :title="t('assistant.settings.model.clear')"
              @click="clearDefaultModel"
            >
              <Trash2 class="size-4" />
            </Button>
          </div>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="loading" @click="handleClose">
          {{ t('assistant.actions.cancel') }}
        </Button>
        <Button :disabled="!isValid || loading" @click="handleCreate">
          {{ t('assistant.actions.create') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
