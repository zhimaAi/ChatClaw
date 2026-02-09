<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { Eye, EyeOff, LoaderCircle, Plus, Pencil, Trash2 } from 'lucide-vue-next'
import ModelIcon from '@/assets/icons/model.svg'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import type {
  Provider,
  ProviderWithModels,
  Model,
} from '@/../bindings/willchat/internal/services/providers'
import {
  ProvidersService,
  UpdateProviderInput,
  CheckAPIKeyInput,
  CreateModelInput,
  UpdateModelInput,
} from '@/../bindings/willchat/internal/services/providers'
import ModelFormDialog from './ModelFormDialog.vue'
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
import { SettingsService } from '@/../bindings/willchat/internal/services/settings'
import { AgentsService } from '@bindings/willchat/internal/services/agents'

// Azure extra_config 类型
interface AzureExtraConfig {
  api_version?: string
}

const props = defineProps<{
  providerWithModels: ProviderWithModels | null
  loading?: boolean
  errorMessage?: string | null
}>()

const emit = defineEmits<{
  update: [provider: Provider]
  refresh: []
}>()

const { t } = useI18n()

// 本地表单状态
const localEnabled = ref(false)
const localApiKey = ref('')
const localApiEndpoint = ref('')
const localApiVersion = ref('') // Azure 专用
const isSaving = ref(false)
const showApiKey = ref(false)

// 检测相关状态
const isChecking = ref(false)

// 判断是否为 Azure（Azure 需要额外的配置）
const isAzure = computed(() => props.providerWithModels?.provider.provider_id === 'azure')

// 判断是否为 Ollama（Ollama 不需要 API Key）
const isOllama = computed(() => props.providerWithModels?.provider.provider_id === 'ollama')

// 判断是否为 ChatWiki（ChatWiki 支持一键生成密钥）
const isChatWiki = computed(() => props.providerWithModels?.provider.provider_id === 'chatwiki')

// 检测按钮是否禁用
const isCheckDisabled = computed(
  () => isSaving.value || isChecking.value || (!isOllama.value && !localApiKey.value.trim())
)

// 生成 ChatWiki API 密钥
const isGeneratingKey = ref(false)
const handleGenerateChatWikiKey = async () => {
  if (!props.providerWithModels || props.providerWithModels.provider.provider_id !== 'chatwiki')
    return

  isGeneratingKey.value = true
  try {
    const key = await ProvidersService.GenerateChatWikiAPIKey()
    localApiKey.value = key
    // Auto-save
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ api_key: key })
    )
    if (updated) {
      emit('update', updated)
      toast.success(t('settings.modelService.generateApiKeySuccess'))
    }
  } catch (error) {
    console.error('Failed to generate ChatWiki API key:', error)
    toast.error(getErrorMessage(error))
  } finally {
    isGeneratingKey.value = false
  }
}

// 解析 extra_config
const parseExtraConfig = (configStr: string): AzureExtraConfig => {
  try {
    return configStr ? JSON.parse(configStr) : {}
  } catch {
    return {}
  }
}

// 监听 props 变化，同步本地状态
watch(
  () => props.providerWithModels?.provider,
  (provider) => {
    if (provider) {
      localEnabled.value = provider.enabled
      localApiKey.value = provider.api_key
      localApiEndpoint.value = provider.api_endpoint
      // 切换供应商时重置密钥显示状态
      showApiKey.value = false
      // 解析 Azure 的额外配置
      if (provider.provider_id === 'azure') {
        const extraConfig = parseExtraConfig(provider.extra_config)
        localApiVersion.value = extraConfig.api_version || ''
      } else {
        localApiVersion.value = ''
      }
    }
  },
  { immediate: true }
)

// 表单验证
const isFormValid = computed(() => {
  if (!props.providerWithModels) return false

  // Ollama 不需要 API Key；ChatWiki 应用初始化时自动生成密钥，无需检测
  if (isOllama.value || isChatWiki.value) {
    return true
  }

  // 必须填写 API Key
  if (!localApiKey.value.trim()) {
    return false
  }

  // Azure 需要额外验证
  if (isAzure.value) {
    // Azure 必须填写 API 地址和 API 版本
    if (!localApiEndpoint.value.trim()) {
      return false
    }
    if (!localApiVersion.value.trim()) {
      return false
    }
  }

  return true
})

// 获取验证提示信息
const validationMessage = computed(() => {
  if (!props.providerWithModels) return ''

  if (isOllama.value || isChatWiki.value) return ''

  if (!localApiKey.value.trim()) {
    return t('settings.modelService.apiKeyRequired')
  }

  if (isAzure.value) {
    if (!localApiEndpoint.value.trim()) {
      return t('settings.modelService.apiEndpointRequired')
    }
    if (!localApiVersion.value.trim()) {
      return t('settings.modelService.apiVersionRequired')
    }
  }

  return ''
})

const isUsedByGlobalEmbedding = async (providerId: string): Promise<boolean> => {
  try {
    const [p, m] = await Promise.all([
      SettingsService.Get('embedding_provider_id'),
      SettingsService.Get('embedding_model_id'),
    ])
    const embeddingProviderId = p?.value?.trim() || ''
    const embeddingModelId = m?.value?.trim() || ''
    return embeddingProviderId === providerId && embeddingModelId !== ''
  } catch (error) {
    console.error('Failed to read embedding settings:', error)
    return false
  }
}

// 检查 provider 是否被助手用作默认模型
const isUsedByAgentDefaultModel = async (providerId: string): Promise<string | null> => {
  try {
    const agents = await AgentsService.ListAgents()
    const agent = agents.find(
      (a) => a.default_llm_provider_id === providerId && a.default_llm_model_id
    )
    return agent?.name ?? null
  } catch (error) {
    console.error('Failed to check agent default models:', error)
    return null
  }
}

// 检查模型是否被助手用作默认模型
const isModelUsedByAgent = async (providerId: string, modelId: string): Promise<string | null> => {
  try {
    const agents = await AgentsService.ListAgents()
    const agent = agents.find(
      (a) => a.default_llm_provider_id === providerId && a.default_llm_model_id === modelId
    )
    return agent?.name ?? null
  } catch (error) {
    console.error('Failed to check agent default models:', error)
    return null
  }
}

// 获取模型组的翻译标题
const getModelGroupTitle = (type: string) => {
  switch (type) {
    case 'llm':
      return t('settings.modelService.llmModels')
    case 'embedding':
      return t('settings.modelService.embeddingModels')
    case 'rerank':
      return t('settings.modelService.rerankModels')
    default:
      return type
  }
}

// 处理开关切换
const handleToggle = async (checked: boolean) => {
  if (!props.providerWithModels) return

  // 如果要启用，需要验证表单
  if (checked && !isFormValid.value) {
    // 不允许启用，保持关闭状态
    return
  }

  // 如果要关闭，需要检查是否被使用（ChatWiki 关闭时无需验证）
  if (!checked) {
    const pid = props.providerWithModels.provider.provider_id
    if (pid === 'chatwiki') {
      // ChatWiki: allow disable without validation
      localEnabled.value = checked
      void saveEnabled(checked)
      return
    }

    // 检查是否被全局嵌入模型使用
    if (await isUsedByGlobalEmbedding(pid)) {
      toast.error(t('settings.modelService.disableBlockedByEmbedding'))
      // 保持开启
      localEnabled.value = true
      return
    }

    // 检查是否被助手默认模型使用
    const agentName = await isUsedByAgentDefaultModel(pid)
    if (agentName) {
      toast.error(t('settings.modelService.disableBlockedByAgent', { name: agentName }))
      // 保持开启
      localEnabled.value = true
      return
    }
  }

  // 更新本地状态
  localEnabled.value = checked
  // 异步保存
  void saveEnabled(checked)
}

// 保存启用状态
const saveEnabled = async (enabled: boolean) => {
  if (!props.providerWithModels) return

  isSaving.value = true
  try {
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ enabled })
    )
    if (updated) {
      emit('update', updated)
    }
  } catch (error) {
    console.error('Failed to update provider:', error)
    toast.error(getErrorMessage(error))
    // 回滚本地状态
    localEnabled.value = props.providerWithModels.provider.enabled
  } finally {
    isSaving.value = false
  }
}

// 处理 API Key 保存（失焦时）
const handleApiKeyBlur = async () => {
  if (!props.providerWithModels) return
  if (localApiKey.value === props.providerWithModels.provider.api_key) return

  isSaving.value = true
  try {
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ api_key: localApiKey.value })
    )
    if (updated) {
      emit('update', updated)
    }
  } catch (error) {
    console.error('Failed to update API key:', error)
    localApiKey.value = props.providerWithModels.provider.api_key
  } finally {
    isSaving.value = false
  }
}

// 切换密钥显示/隐藏
const toggleShowApiKey = () => {
  showApiKey.value = !showApiKey.value
}

// 处理 API Key 检测
const handleCheck = async () => {
  if (!props.providerWithModels) return

  isChecking.value = true
  try {
    // 构建 extra_config
    let extraConfig = ''
    if (isAzure.value && localApiVersion.value) {
      extraConfig = JSON.stringify({ api_version: localApiVersion.value })
    }

    const result = await ProvidersService.CheckAPIKey(
      props.providerWithModels.provider.provider_id,
      new CheckAPIKeyInput({
        api_key: localApiKey.value,
        api_endpoint: localApiEndpoint.value,
        extra_config: extraConfig,
      })
    )
    if (result?.success) {
      toast.success(t('settings.modelService.checkSuccess'))
    } else {
      toast.error(result?.message || t('settings.modelService.checkFailed'))
    }
  } catch (error) {
    console.error('Failed to check API key:', error)
    toast.error(getErrorMessage(error))
  } finally {
    isChecking.value = false
  }
}

// 处理 API Endpoint 保存（失焦时）
const handleApiEndpointBlur = async () => {
  if (!props.providerWithModels) return
  if (localApiEndpoint.value === props.providerWithModels.provider.api_endpoint) return

  isSaving.value = true
  try {
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ api_endpoint: localApiEndpoint.value })
    )
    if (updated) {
      emit('update', updated)
    }
  } catch (error) {
    console.error('Failed to update API endpoint:', error)
    localApiEndpoint.value = props.providerWithModels.provider.api_endpoint
  } finally {
    isSaving.value = false
  }
}

// 处理 API Version 保存（Azure 专用，失焦时）
const handleApiVersionBlur = async () => {
  if (!props.providerWithModels) return

  const currentConfig = parseExtraConfig(props.providerWithModels.provider.extra_config)
  if (localApiVersion.value === (currentConfig.api_version || '')) return

  isSaving.value = true
  try {
    const newConfig: AzureExtraConfig = {
      ...currentConfig,
      api_version: localApiVersion.value,
    }
    const updated = await ProvidersService.UpdateProvider(
      props.providerWithModels.provider.provider_id,
      new UpdateProviderInput({ extra_config: JSON.stringify(newConfig) })
    )
    if (updated) {
      emit('update', updated)
    }
  } catch (error) {
    console.error('Failed to update API version:', error)
    // 回滚本地状态（需要检查 props 是否仍然有效）
    if (props.providerWithModels) {
      const fallbackConfig = parseExtraConfig(props.providerWithModels.provider.extra_config)
      localApiVersion.value = fallbackConfig.api_version || ''
    }
  } finally {
    isSaving.value = false
  }
}

// 处理重置 API Endpoint
const handleResetEndpoint = async () => {
  if (!props.providerWithModels) return

  isSaving.value = true
  try {
    const updated = await ProvidersService.ResetAPIEndpoint(
      props.providerWithModels.provider.provider_id
    )
    if (updated) {
      localApiEndpoint.value = updated.api_endpoint
      emit('update', updated)
    }
  } catch (error) {
    console.error('Failed to reset API endpoint:', error)
  } finally {
    isSaving.value = false
  }
}

// 默认展开的手风琴项
const defaultAccordionValue = computed(() => {
  const groups = props.providerWithModels?.model_groups || []
  return groups.map((g) => g.type)
})

// 模型对话框相关状态
const modelDialogOpen = ref(false)
const editingModel = ref<Model | null>(null)
const modelFormDialogRef = ref<InstanceType<typeof ModelFormDialog> | null>(null)

// 打开添加模型对话框
const handleAddModel = () => {
  editingModel.value = null
  modelDialogOpen.value = true
}

// 打开编辑模型对话框
const handleEditModel = (model: Model) => {
  editingModel.value = model
  modelDialogOpen.value = true
}

// 保存模型（添加或编辑）
const handleSaveModel = async (data: { modelId: string; name: string; type: string }) => {
  if (!props.providerWithModels) return

  try {
    if (editingModel.value) {
      // 编辑模式（只允许修改 name）
      await ProvidersService.UpdateModel(
        props.providerWithModels.provider.provider_id,
        editingModel.value.model_id,
        new UpdateModelInput({
          name: data.name,
        })
      )
      toast.success(t('settings.modelService.modelUpdated'))
    } else {
      // 添加模式
      await ProvidersService.CreateModel(
        props.providerWithModels.provider.provider_id,
        new CreateModelInput({
          model_id: data.modelId,
          name: data.name,
          type: data.type,
        })
      )
      toast.success(t('settings.modelService.modelCreated'))
    }

    modelDialogOpen.value = false
    // 触发刷新模型列表
    emit('refresh')
  } catch (error) {
    console.error('Failed to save model:', error)
    toast.error(getErrorMessage(error))
  } finally {
    modelFormDialogRef.value?.resetSaving()
  }
}

// 删除确认对话框相关状态
const deleteDialogOpen = ref(false)
const deletingModel = ref<Model | null>(null)
const isDeleting = ref(false)

// 打开删除确认对话框
const handleDeleteModel = (model: Model) => {
  deletingModel.value = model
  deleteDialogOpen.value = true
}

// 确认删除模型
const confirmDeleteModel = async () => {
  if (!props.providerWithModels || !deletingModel.value) return

  isDeleting.value = true
  try {
    const providerId = props.providerWithModels.provider.provider_id
    const modelId = deletingModel.value.model_id

    // 检查模型是否被助手用作默认模型
    const agentName = await isModelUsedByAgent(providerId, modelId)
    if (agentName) {
      toast.error(t('settings.modelService.deleteBlockedByAgent', { name: agentName }))
      deleteDialogOpen.value = false
      return
    }

    await ProvidersService.DeleteModel(providerId, modelId)
    toast.success(t('settings.modelService.modelDeleted'))
    deleteDialogOpen.value = false
    // 触发刷新模型列表
    emit('refresh')
  } catch (error) {
    console.error('Failed to delete model:', error)
    toast.error(getErrorMessage(error))
  } finally {
    isDeleting.value = false
  }
}
</script>

<template>
  <div class="flex flex-1 flex-col overflow-auto p-6">
    <!-- 加载状态 -->
    <div v-if="loading" class="flex items-center justify-center py-16">
      <div class="size-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
    </div>

    <!-- 空状态 -->
    <div
      v-else-if="!providerWithModels"
      class="flex items-center justify-center py-16 text-muted-foreground"
    >
      {{ t('settings.modelService.loadingProviders') }}
    </div>

    <!-- 详情内容 -->
    <div v-else class="mx-auto w-full max-w-settings-card">
      <div
        class="rounded-xl border border-border bg-card p-6 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
      >
        <!-- Error banner (models failed to load, but form still usable) -->
        <div
          v-if="props.errorMessage"
          class="mb-4 flex items-start justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3"
        >
          <div class="min-w-0 text-sm text-muted-foreground">
            {{ t('settings.modelService.loadFailed') }}：{{ props.errorMessage }}
          </div>
          <Button size="sm" variant="outline" :disabled="loading" @click="emit('refresh')">
            {{ t('common.retry') }}
          </Button>
        </div>

        <!-- 标题和开关 -->
        <div
          class="flex items-center justify-between rounded-lg border border-border bg-background/50 px-3 py-3 dark:border-white/10"
        >
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-foreground">
              {{ providerWithModels.provider.name }}
            </span>
          </div>
          <div class="flex items-center gap-2">
            <!-- 验证提示 -->
            <span v-if="!localEnabled && validationMessage" class="text-xs text-muted-foreground">
              {{ validationMessage }}
            </span>
            <Switch
              :model-value="localEnabled"
              :disabled="isSaving || (!localEnabled && !isFormValid)"
              @update:model-value="handleToggle"
            />
          </div>
        </div>

        <!-- 表单区域 -->
        <div class="mt-6 flex flex-col gap-4">
          <!-- API 密钥 -->
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-foreground">
              {{ t('settings.modelService.apiKey') }}
              <span v-if="!isOllama" class="text-destructive">*</span>
            </label>
            <div class="flex gap-2">
              <div class="relative flex-1">
                <Input
                  v-model="localApiKey"
                  :type="showApiKey ? 'text' : 'password'"
                  :placeholder="t('settings.modelService.apiKeyPlaceholder')"
                  class="pr-10"
                  :disabled="isSaving"
                  @blur="handleApiKeyBlur"
                />
                <button
                  type="button"
                  class="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                  @click="toggleShowApiKey"
                >
                  <Eye v-if="!showApiKey" class="size-4" />
                  <EyeOff v-else class="size-4" />
                </button>
              </div>
              <Button
                v-if="isChatWiki"
                variant="outline"
                :disabled="isGeneratingKey || isSaving"
                class="min-w-[72px]"
                @click="handleGenerateChatWikiKey"
              >
                <LoaderCircle v-if="isGeneratingKey" class="size-4 animate-spin" />
                <span v-else>{{ t('settings.modelService.generateApiKey') }}</span>
              </Button>
              <Button
                v-if="!isChatWiki"
                variant="outline"
                :disabled="isCheckDisabled"
                class="min-w-[72px]"
                @click="handleCheck"
              >
                <LoaderCircle v-if="isChecking" class="size-4 animate-spin" />
                <span v-else>{{ t('settings.modelService.check') }}</span>
              </Button>
            </div>
          </div>

          <!-- API 地址 -->
          <div class="flex flex-col gap-1.5">
            <div class="flex items-center gap-1">
              <label class="text-sm font-medium text-foreground">
                {{ t('settings.modelService.apiEndpoint') }}
                <span v-if="isAzure" class="text-destructive">*</span>
              </label>
            </div>
            <div class="flex gap-2">
              <Input
                v-model="localApiEndpoint"
                type="text"
                :placeholder="t('settings.modelService.apiEndpointPlaceholder')"
                class="flex-1"
                :disabled="isSaving"
                @blur="handleApiEndpointBlur"
              />
              <Button
                variant="outline"
                class="min-w-[72px]"
                :disabled="isSaving"
                @click="handleResetEndpoint"
              >
                {{ t('settings.modelService.reset') }}
              </Button>
            </div>
          </div>

          <!-- Azure API Version -->
          <div v-if="isAzure" class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-foreground">
              {{ t('settings.modelService.apiVersion') }}
              <span class="text-destructive">*</span>
            </label>
            <Input
              v-model="localApiVersion"
              type="text"
              :placeholder="t('settings.modelService.apiVersionPlaceholder')"
              :disabled="isSaving"
              @blur="handleApiVersionBlur"
            />
          </div>

          <!-- 添加模型按钮（ChatWiki 模型仅通过接口获取，不支持添加） -->
          <div v-if="!isChatWiki" class="flex">
            <Button variant="outline" size="sm" class="gap-1.5" @click="handleAddModel">
              <Plus class="size-4" />
              {{ t('settings.modelService.addModel') }}
            </Button>
          </div>

          <!-- 模型列表 -->
          <div class="flex flex-col gap-1.5">
            <div class="overflow-hidden rounded-md border border-border dark:border-white/10">
              <Accordion type="multiple" :default-value="defaultAccordionValue" class="w-full">
                <AccordionItem
                  v-for="group in providerWithModels.model_groups"
                  :key="group.type"
                  :value="group.type"
                  class="border-b border-border last:border-b-0 dark:border-white/10"
                >
                  <AccordionTrigger class="px-4 hover:no-underline">
                    {{ getModelGroupTitle(group.type) }}
                  </AccordionTrigger>
                  <AccordionContent>
                    <div class="flex flex-col">
                      <div
                        v-for="model in group.models"
                        :key="model.model_id"
                        class="group flex items-center gap-2 px-4 py-2 hover:bg-accent/50"
                      >
                        <ModelIcon class="size-5 shrink-0 text-muted-foreground" />
                        <span class="min-w-0 flex-1 truncate text-sm text-foreground">{{
                          model.name
                        }}</span>
                        <!-- 编辑和删除按钮（仅对非内置模型显示，ChatWiki 模型禁止编辑删除） -->
                        <div
                          v-if="!isChatWiki && !model.is_builtin"
                          class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100"
                        >
                          <button
                            class="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
                            :title="t('settings.modelService.editModel')"
                            @click.stop="handleEditModel(model)"
                          >
                            <Pencil class="size-3.5" />
                          </button>
                          <button
                            class="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                            :title="t('settings.modelService.deleteModel')"
                            @click.stop="handleDeleteModel(model)"
                          >
                            <Trash2 class="size-3.5" />
                          </button>
                        </div>
                      </div>
                      <div
                        v-if="group.models.length === 0"
                        class="px-4 py-2 text-sm text-muted-foreground"
                      >
                        {{ t('settings.modelService.noModels') }}
                      </div>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 模型表单对话框 -->
    <ModelFormDialog
      ref="modelFormDialogRef"
      v-model:open="modelDialogOpen"
      :model="editingModel"
      :provider-name="providerWithModels?.provider.name || ''"
      @save="handleSaveModel"
    />

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.modelService.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.modelService.deleteConfirmMessage', { name: deletingModel?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel :disabled="isDeleting">
            {{ t('settings.modelService.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            :disabled="isDeleting"
            @click.prevent="confirmDeleteModel"
          >
            {{
              isDeleting
                ? t('settings.modelService.deleting')
                : t('settings.modelService.confirmDelete')
            }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
