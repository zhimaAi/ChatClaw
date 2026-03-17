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

export const CHATWIKI_PROVIDER_ID = 'chatwiki'

export function isModelSelectionDisabled(providerId: string, isChatwikiBound: boolean): boolean {
  return providerId === CHATWIKI_PROVIDER_ID && !isChatwikiBound
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
  isChatwikiBound: boolean
): string {
  const parsed = parseModelSelectionKey(key)
  if (!parsed) return ''
  return isModelSelectionDisabled(parsed.providerId, isChatwikiBound) ? '' : key
}

export function isSelectionAvailable<TModel extends ModelLike>(
  providersWithModels: Array<ProviderWithModelsLike<TModel>>,
  key: string,
  groupType: string,
  isChatwikiBound: boolean
): boolean {
  const parsed = parseModelSelectionKey(key)
  if (!parsed) return false
  if (isModelSelectionDisabled(parsed.providerId, isChatwikiBound)) return false

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
  isChatwikiBound: boolean
): string {
  for (const providerWithModels of providersWithModels) {
    const providerId = providerWithModels.provider.provider_id
    if (isModelSelectionDisabled(providerId, isChatwikiBound)) continue
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
