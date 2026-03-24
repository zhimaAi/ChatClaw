type ChatwikiBindingLike = {
  chatwiki_version?: string | null
} | null

export type NormalizedChatwikiAvailability = 'available' | 'unbound' | 'non_cloud'

export function normalizeChatwikiVersion(value?: string | null): string {
  return value?.trim().toLowerCase() || ''
}

export function isChatwikiDevBinding(binding: ChatwikiBindingLike): boolean {
  return normalizeChatwikiVersion(binding?.chatwiki_version) === 'dev'
}

export function getNormalizedChatwikiAvailability(
  binding: ChatwikiBindingLike
): NormalizedChatwikiAvailability {
  if (!binding) return 'unbound'
  return isChatwikiDevBinding(binding) ? 'non_cloud' : 'available'
}
