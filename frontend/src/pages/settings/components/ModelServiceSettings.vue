<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ProviderList from './ProviderList.vue'
import ProviderDetail from './ProviderDetail.vue'
import { getErrorMessage } from '@/composables/useErrorMessage'
import type {
  Provider,
  ProviderWithModels,
} from '@/../bindings/willchat/internal/services/providers'
import { ProvidersService } from '@/../bindings/willchat/internal/services/providers'

const { t } = useI18n()

// 状态
const providers = ref<Provider[]>([])
const selectedProviderId = ref<string | null>(null)
const providerWithModels = ref<ProviderWithModels | null>(null)
const loadingProviders = ref(false)
const loadingDetail = ref(false)
const loadError = ref<string | null>(null)
const detailError = ref<string | null>(null)

// 加载供应商列表
const loadProviders = async () => {
  loadingProviders.value = true
  loadError.value = null
  try {
    const list = await ProvidersService.ListProviders()
    providers.value = list || []
    // 默认选中第一个
    if (providers.value.length > 0 && !selectedProviderId.value) {
      selectedProviderId.value = providers.value[0].provider_id
    }
  } catch (error) {
    console.error('Failed to load providers:', error)
    loadError.value = t('settings.modelService.loadFailed')
  } finally {
    loadingProviders.value = false
  }
}

// 加载供应商详情（包含模型）
const loadProviderDetail = async (providerId: string) => {
  loadingDetail.value = true
  detailError.value = null
  try {
    const detail = await ProvidersService.GetProviderWithModels(providerId)
    providerWithModels.value = detail
  } catch (error) {
    console.error('Failed to load provider detail:', error)
    // Fallback: still render provider base info so user can edit endpoint/key.
    // Model list may fail due to invalid endpoint/network; shouldn't block settings UI.
    detailError.value = getErrorMessage(error) || t('settings.modelService.loadFailed')
    try {
      const provider = await ProvidersService.GetProvider(providerId)
      if (provider) {
        providerWithModels.value = { provider, model_groups: [] } as ProviderWithModels
        return
      }
    } catch (e) {
      console.error('Failed to load provider base info:', e)
    }
    providerWithModels.value = null
  } finally {
    loadingDetail.value = false
  }
}

// 监听选中的供应商变化
watch(selectedProviderId, (newId) => {
  if (newId) {
    void loadProviderDetail(newId)
  } else {
    providerWithModels.value = null
  }
})

// 处理供应商选择
const handleProviderSelect = (providerId: string) => {
  selectedProviderId.value = providerId
}

// 处理供应商更新
const handleProviderUpdate = (updated: Provider) => {
  // 更新列表中的供应商
  const index = providers.value.findIndex((p) => p.provider_id === updated.provider_id)
  if (index !== -1) {
    providers.value[index] = updated
  }
  // 更新详情中的供应商
  if (
    providerWithModels.value &&
    providerWithModels.value.provider.provider_id === updated.provider_id
  ) {
    providerWithModels.value = {
      ...providerWithModels.value,
      provider: updated,
    }
  }
}

// 处理模型列表刷新
const handleRefresh = () => {
  if (selectedProviderId.value) {
    void loadProviderDetail(selectedProviderId.value)
  }
}

// 组件挂载时加载数据
onMounted(() => {
  void loadProviders()
})
</script>

<template>
  <div class="flex h-full w-full">
    <!-- 左侧供应商列表 -->
    <ProviderList
      :providers="providers"
      :selected-provider-id="selectedProviderId"
      :loading="loadingProviders"
      @select="handleProviderSelect"
    />

    <!-- 右侧详情区域 -->
    <ProviderDetail
      :provider-with-models="providerWithModels"
      :loading="loadingDetail"
      :error-message="detailError"
      @update="handleProviderUpdate"
      @refresh="handleRefresh"
    />
  </div>
</template>
