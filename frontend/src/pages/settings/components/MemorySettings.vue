<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getErrorMessage } from '@/composables/useErrorMessage'

import type { Provider, Model } from '@bindings/chatclaw/internal/services/providers'
import { ProvidersService } from '@bindings/chatclaw/internal/services/providers'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const showRebuildConfirm = ref(false)

const memoryEnabled = ref(false)
const extractSelectedKey = ref('')
const embeddingSelectedKey = ref('')
const embeddingDimension = ref('1536')

// Saved values for change detection
const savedEmbeddingKey = ref('')
const savedEmbeddingDimension = ref('1536')

type Group = { provider: Provider; models: Model[] }
const extractGroups = ref<Group[]>([])
const embeddingGroups = ref<Group[]>([])

const loadData = async () => {
  loading.value = true
  try {
    const providers = (await ProvidersService.ListProviders()) || []
    const enabledProviders = providers.filter((p) => p.enabled)

    const details = await Promise.all(
      enabledProviders.map(async (p) => {
        try {
          const detail = await ProvidersService.GetProviderWithModels(p.provider_id)
          return { provider: p, detail }
        } catch (error) {
          console.warn(`Failed to load provider ${p.provider_id}:`, error)
          return { provider: p, detail: null }
        }
      })
    )

    const extGroups: Group[] = []
    const embGroups: Group[] = []

    for (const item of details) {
      if (!item.detail) continue

      const llmGroup = item.detail.model_groups?.find((g) => g.type === 'llm')
      const llmModels = (llmGroup?.models || []).filter((m) => m.enabled)
      if (llmModels.length > 0) {
        extGroups.push({ provider: item.provider, models: llmModels })
      }

      const embGroup = item.detail.model_groups?.find((g) => g.type === 'embedding')
      const embModels = (embGroup?.models || []).filter((m) => m.enabled)
      if (embModels.length > 0) {
        embGroups.push({ provider: item.provider, models: embModels })
      }
    }

    extractGroups.value = extGroups
    embeddingGroups.value = embGroups

    const [enabled, extProv, extMod, embProv, embMod, embDim] = await Promise.all([
      SettingsService.Get('memory_enabled'),
      SettingsService.Get('memory_extract_provider_id'),
      SettingsService.Get('memory_extract_model_id'),
      SettingsService.Get('memory_embedding_provider_id'),
      SettingsService.Get('memory_embedding_model_id'),
      SettingsService.Get('memory_embedding_dimension'),
    ])

    memoryEnabled.value = enabled?.value === 'true'

    if (extProv?.value && extMod?.value) {
      extractSelectedKey.value = `${extProv.value}::${extMod.value}`
    }

    if (embProv?.value && embMod?.value) {
      const key = `${embProv.value}::${embMod.value}`
      embeddingSelectedKey.value = key
      savedEmbeddingKey.value = key
    }

    if (embDim?.value) {
      embeddingDimension.value = embDim.value
      savedEmbeddingDimension.value = embDim.value
    }
  } catch (error) {
    console.error('Failed to load memory settings:', error)
    toast.error(getErrorMessage(error) || t('settings.memory.saveFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadData()
})

const isValid = computed(() => {
  if (!memoryEnabled.value) return true
  if (!extractSelectedKey.value || !embeddingSelectedKey.value) return false
  const dim = Number.parseInt(embeddingDimension.value, 10)
  return Number.isFinite(dim) && dim > 0
})

const embeddingChanged = computed(() => {
  return (
    embeddingSelectedKey.value !== savedEmbeddingKey.value ||
    embeddingDimension.value !== savedEmbeddingDimension.value
  )
})

const handleSave = () => {
  if (!isValid.value || saving.value) return

  if (memoryEnabled.value && embeddingChanged.value && savedEmbeddingKey.value !== '') {
    showRebuildConfirm.value = true
    return
  }

  doSave()
}

const doSave = async () => {
  saving.value = true

  try {
    const [extProv, extMod] = extractSelectedKey.value.split('::')
    const [embProv, embMod] = embeddingSelectedKey.value.split('::')

    await SettingsService.UpdateMemorySettings({
      enabled: memoryEnabled.value,
      extract_provider_id: extProv || '',
      extract_model_id: extMod || '',
      embedding_provider_id: embProv || '',
      embedding_model_id: embMod || '',
      embedding_dimension: Number.parseInt(embeddingDimension.value || '1536', 10),
    })

    savedEmbeddingKey.value = embeddingSelectedKey.value
    savedEmbeddingDimension.value = embeddingDimension.value

    toast.success(t('settings.memory.saved'))
  } catch (error) {
    console.error('Failed to save memory settings:', error)
    toast.error(getErrorMessage(error) || t('settings.memory.saveFailed'))
  } finally {
    saving.value = false
  }
}

const confirmRebuild = () => {
  showRebuildConfirm.value = false
  doSave()
}

function isProviderFree(g: Group): boolean {
  const p = g.provider as { is_free?: boolean }
  return Boolean(p?.is_free)
}
</script>

<template>
  <div
    class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
  >
    <div class="flex flex-col gap-1.5">
      <h2 class="text-lg font-semibold tracking-tight">{{ t('settings.memory.title') }}</h2>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-8">
      <LoaderCircle class="size-6 animate-spin text-muted-foreground" />
    </div>

    <template v-else>
      <!-- Enable Switch -->
      <div class="flex items-center justify-between">
        <div class="flex flex-col gap-1">
          <span class="text-sm font-medium">{{ t('settings.memory.enable') }}</span>
          <span class="text-xs text-muted-foreground">{{ t('settings.memory.enableHint') }}</span>
        </div>
        <Switch v-model="memoryEnabled" />
      </div>

      <div class="flex flex-col gap-6 border-t border-border pt-6 dark:border-white/10">
        <!-- Extract Model -->
        <div class="flex flex-col gap-2">
          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium">{{ t('settings.memory.extractModel') }}</span>
            <span class="text-xs text-muted-foreground">{{
              t('settings.memory.extractModelHint')
            }}</span>
          </div>
          <Select v-model="extractSelectedKey" :disabled="saving">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup v-for="g in extractGroups" :key="g.provider.provider_id">
                <SelectLabel class="flex items-center gap-1.5">
                  <span>{{ g.provider.name }}</span>
                  <span
                    v-if="isProviderFree(g)"
                    class="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                  >
                    {{ t('assistant.chat.freeBadge') }}
                  </span>
                </SelectLabel>
                <SelectItem
                  v-for="m in g.models"
                  :key="`${g.provider.provider_id}::${m.model_id}`"
                  :value="`${g.provider.provider_id}::${m.model_id}`"
                >
                  {{ m.name }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <!-- Embedding Model -->
        <div class="flex flex-col gap-2">
          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium">{{ t('settings.memory.embeddingModel') }}</span>
            <span class="text-xs text-muted-foreground">{{
              t('settings.memory.embeddingModelHint')
            }}</span>
          </div>
          <Select v-model="embeddingSelectedKey" :disabled="saving">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('knowledge.create.selectPlaceholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup v-for="g in embeddingGroups" :key="g.provider.provider_id">
                <SelectLabel class="flex items-center gap-1.5">
                  <span>{{ g.provider.name }}</span>
                  <span
                    v-if="isProviderFree(g)"
                    class="rounded px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground ring-1 ring-border"
                  >
                    {{ t('assistant.chat.freeBadge') }}
                  </span>
                </SelectLabel>
                <SelectItem
                  v-for="m in g.models"
                  :key="`${g.provider.provider_id}::${m.model_id}`"
                  :value="`${g.provider.provider_id}::${m.model_id}`"
                >
                  {{ m.name }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <!-- Embedding Dimension -->
        <div class="flex flex-col gap-2">
          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium">{{ t('settings.memory.embeddingDimension') }}</span>
            <span class="text-xs text-muted-foreground">{{
              t('settings.memory.embeddingDimensionHint')
            }}</span>
          </div>
          <Input v-model="embeddingDimension" type="number" min="1" step="1" :disabled="saving" />
        </div>
      </div>

      <div class="flex justify-end">
        <Button class="gap-2" :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="size-4 shrink-0 animate-spin" />
          {{ t('settings.memory.save') }}
        </Button>
      </div>
    </template>

    <AlertDialog :open="showRebuildConfirm" @update:open="showRebuildConfirm = $event">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.memory.confirmRebuildTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>{{
            t('settings.memory.confirmRebuildDesc')
          }}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel @click="showRebuildConfirm = false">{{
            t('common.cancel')
          }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-foreground text-background hover:bg-foreground/90"
            @click="confirmRebuild"
          >
            {{ t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
