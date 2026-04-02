import { ProvidersService } from '@bindings/chatclaw/internal/services/providers'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import { getBinding as getChatwikiBinding } from '@/lib/chatwikiCache'
import { getChatwikiAvailabilityStatus } from '@/lib/chatwikiModelAvailability'

type AzureExtraConfig = {
  api_version?: string
}

function parseAzureExtraConfig(configStr: string): AzureExtraConfig {
  try {
    return configStr ? JSON.parse(configStr) : {}
  } catch {
    return {}
  }
}

export async function isGlobalEmbeddingConfigReady(): Promise<boolean> {
  const [providerSetting, modelSetting] = await Promise.all([
    SettingsService.Get('embedding_provider_id'),
    SettingsService.Get('embedding_model_id'),
  ])

  const providerId = providerSetting?.value?.trim() || ''
  const modelId = modelSetting?.value?.trim() || ''
  if (!providerId || !modelId) return false

  const [providerWithModels, binding] = await Promise.all([
    ProvidersService.GetProviderWithModels(providerId).catch(() => null),
    getChatwikiBinding().catch(() => null),
  ])
  if (!providerWithModels?.provider?.enabled) return false

  const embeddingGroup = providerWithModels.model_groups?.find((group) => group.type === 'embedding')
  const model = embeddingGroup?.models?.find((item) => item.model_id === modelId && item.enabled)
  if (!model) return false

  const provider = providerWithModels.provider
  if (providerId === 'chatwiki') {
    return (
      getChatwikiAvailabilityStatus(binding) === 'available' &&
      !!provider.api_key?.trim() &&
      !!provider.api_endpoint?.trim()
    )
  }

  if (provider.type === 'ollama') {
    return true
  }

  if (!provider.api_key?.trim()) {
    return false
  }

  if (provider.type === 'azure') {
    const extraConfig = parseAzureExtraConfig(provider.extra_config)
    return !!provider.api_endpoint?.trim() && !!extraConfig.api_version?.trim()
  }

  return true
}
