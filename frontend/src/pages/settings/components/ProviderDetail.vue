<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff } from 'lucide-vue-next'
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
import { cn } from '@/lib/utils'
import type {
  Provider,
  ProviderWithModels,
} from '@/../bindings/willchat/internal/services/providers'
import { ProvidersService, UpdateProviderInput } from '@/../bindings/willchat/internal/services/providers'

// Azure extra_config 类型
interface AzureExtraConfig {
  api_version?: string
}

const props = defineProps<{
  providerWithModels: ProviderWithModels | null
  loading?: boolean
}>()

const emit = defineEmits<{
  update: [provider: Provider]
}>()

const { t } = useI18n()

// 本地表单状态
const localEnabled = ref(false)
const localApiKey = ref('')
const localApiEndpoint = ref('')
const localApiVersion = ref('') // Azure 专用
const isSaving = ref(false)
const showApiKey = ref(false)

// 判断是否为 Azure
const isAzure = computed(() => props.providerWithModels?.provider.provider_id === 'azure')

// 判断是否为 Ollama（Ollama 不需要 API Key）
const isOllama = computed(() => props.providerWithModels?.provider.provider_id === 'ollama')

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
  
  // Ollama 不需要 API Key
  if (isOllama.value) {
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
  
  if (isOllama.value) return ''
  
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

// 获取模型组的翻译标题
const getModelGroupTitle = (type: string) => {
  return type === 'llm'
    ? t('settings.modelService.llmModels')
    : t('settings.modelService.embeddingModels')
}

// 处理开关切换
const handleToggle = (checked: boolean) => {
  if (!props.providerWithModels) return

  // 如果要启用，需要验证表单
  if (checked && !isFormValid.value) {
    // 不允许启用，保持关闭状态
    return
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
    <div v-else class="mx-auto w-full max-w-[530px]">
      <div
        class="rounded-xl border border-border bg-card p-6 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
      >
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
            <span
              v-if="!localEnabled && validationMessage"
              class="text-xs text-muted-foreground"
            >
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
            <div class="relative">
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
              <Button variant="outline" :disabled="isSaving" @click="handleResetEndpoint">
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

          <!-- 模型列表 -->
          <div class="flex flex-col gap-1.5">
            <div
              class="overflow-hidden rounded-md border border-border dark:border-white/10"
            >
              <Accordion
                type="multiple"
                :default-value="defaultAccordionValue"
                class="w-full"
              >
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
                        class="flex items-center gap-2 px-4 py-2"
                      >
                        <ModelIcon class="size-5 shrink-0 text-muted-foreground" />
                        <span class="text-sm text-foreground">{{ model.name }}</span>
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
  </div>
</template>
