import { i18n } from '@/i18n'
import { getNormalizedChatwikiAvailability } from './chatwikiBindingVersion'

type ModelLike = {
  model_id: string
  enabled?: boolean
}

type ModelGroupLike<TModel extends ModelLike = ModelLike> = {
  type: string
  models: TModel[]
}

type ProviderWithModelsLike<TModel extends ModelLike = ModelLike> = {
  provider: { provider_id: string }
  model_groups: Array<ModelGroupLike<TModel>>
}

type ChatwikiBindingLike = {
  chatwiki_version?: string | null
} | null

export type ChatwikiAvailabilityStatus = 'available' | 'unbound' | 'non_cloud'

export const CHATWIKI_PROVIDER_ID = 'chatwiki'

export function getChatwikiAvailabilityStatus(
  binding: ChatwikiBindingLike
): ChatwikiAvailabilityStatus {
  return getNormalizedChatwikiAvailability(binding)
}

function normalizeAvailabilityStatus(
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): ChatwikiAvailabilityStatus {
  if (statusOrBinding === true) return 'available'
  if (statusOrBinding === false) return 'unbound'
  if (
    statusOrBinding === 'available' ||
    statusOrBinding === 'unbound' ||
    statusOrBinding === 'non_cloud'
  ) {
    return statusOrBinding
  }
  return getChatwikiAvailabilityStatus(statusOrBinding)
}

export function formatModelDisplayLabel(
  providerId: string,
  label: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): string {
  return label
}

export function formatProviderDisplayLabel(
  providerId: string,
  label: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): string {
  const status = normalizeAvailabilityStatus(statusOrBinding)
  if (providerId !== CHATWIKI_PROVIDER_ID) return label
  if (status === 'unbound') {
    return (i18n.global as any).t('settings.chatwiki.providerStatus.unbound', { label }) as string
  }
  if (status === 'non_cloud') {
    return (i18n.global as any).t('settings.chatwiki.providerStatus.nonCloud', { label }) as string
  }
  return label
}

export function isModelSelectionDisabled(
  providerId: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): boolean {
  return (
    providerId === CHATWIKI_PROVIDER_ID && normalizeAvailabilityStatus(statusOrBinding) !== 'available'
  )
}

export function parseModelSelectionKey(
  key: string
): { providerId: string; modelId: string } | null {
  if (!key) return null
  const [providerId, modelId] = key.split('::')
  if (!providerId || !modelId) return null
  return { providerId, modelId }
}

export function clearUnavailableChatwikiSelection(
  key: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): string {
  const parsed = parseModelSelectionKey(key)
  if (!parsed) return ''
  return isModelSelectionDisabled(parsed.providerId, statusOrBinding) ? '' : key
}

export function isSelectionAvailable<TModel extends ModelLike>(
  providersWithModels: Array<ProviderWithModelsLike<TModel>>,
  key: string,
  groupType: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): boolean {
  const parsed = parseModelSelectionKey(key)
  if (!parsed) return false
  if (isModelSelectionDisabled(parsed.providerId, statusOrBinding)) return false

  for (const providerWithModels of providersWithModels) {
    if (providerWithModels.provider.provider_id !== parsed.providerId) continue
    for (const group of providerWithModels.model_groups) {
      if (group.type !== groupType) continue
      const found = group.models.find(
        (model) => model.model_id === parsed.modelId && model.enabled !== false
      )
      if (found) return true
    }
  }

  return false
}

export function getFirstSelectableModelKey<TModel extends ModelLike>(
  providersWithModels: Array<ProviderWithModelsLike<TModel>>,
  groupType: string,
  statusOrBinding: ChatwikiAvailabilityStatus | boolean | ChatwikiBindingLike
): string {
  for (const providerWithModels of providersWithModels) {
    const providerId = providerWithModels.provider.provider_id
    if (isModelSelectionDisabled(providerId, statusOrBinding)) continue
    for (const group of providerWithModels.model_groups) {
      if (group.type !== groupType) continue
      const firstModel = group.models.find((model) => model.enabled !== false)
      if (firstModel) {
        return `${providerId}::${firstModel.model_id}`
      }
    }
  }

  return ''
}
