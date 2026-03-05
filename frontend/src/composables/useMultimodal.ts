/**
 * Multimodal model detection utilities
 * Checks if a model supports vision/multimodal capabilities
 */

/**
 * Check if a model supports multimodal (vision) capabilities based on provider and model ID
 * This mirrors the backend logic in internal/services/chat/generation.go
 */
export function supportsMultimodal(providerId: string, modelId: string): boolean {
  const providerIdLower = providerId.toLowerCase()
  const modelIdLower = modelId.toLowerCase()

  // OpenAI and Azure models with vision support
  if (providerIdLower === 'openai' || providerIdLower === 'azure') {
    if (modelIdLower.includes('gpt-4') && !modelIdLower.includes('gpt-4o-mini')) {
      return true
    }
    if (modelIdLower.includes('gpt-5')) {
      return true
    }
    if (modelIdLower.includes('vision')) {
      return true
    }
  }

  // Anthropic Claude models support vision
  if (providerIdLower === 'anthropic') {
    if (modelIdLower.includes('claude')) {
      return true
    }
  }

  // Google Gemini models all support vision
  if (providerIdLower === 'google' || providerIdLower === 'gemini') {
    return true
  }

  // Qwen VL (Vision-Language) models support vision
  if (providerIdLower === 'qwen') {
    if (modelIdLower.includes('vl') || modelIdLower.includes('vision')) {
      return true
    }
    // qwen-plus, qwen-max, qwen-flash, qwen-long are text-only
  }

  // DeepSeek models with vision
  if (providerIdLower === 'deepseek') {
    if (modelIdLower.includes('vision')) {
      return true
    }
  }

  // Default: assume text-only
  return false
}
