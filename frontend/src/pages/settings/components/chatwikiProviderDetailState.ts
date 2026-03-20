export type ChatwikiProviderDetailBindingLike = {
  chatwiki_version?: string | null
}

function normalizeText(value?: string | null): string {
  return value?.trim().toLowerCase() || ''
}

export function shouldShowChatwikiAccountCard(
  binding: ChatwikiProviderDetailBindingLike | null
): boolean {
  return binding != null
}

export function shouldShowChatwikiCreditsCard(
  binding: ChatwikiProviderDetailBindingLike | null
): boolean {
  return normalizeText(binding?.chatwiki_version) === 'yun'
}
