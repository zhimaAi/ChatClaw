/**
 * Multimodal model detection utilities
 * Checks if a model supports vision/multimodal capabilities
 */

/**
 * Check if capabilities include a specific type (e.g., "image")
 */
function hasCapability(capabilities: string[] | undefined, capability: string): boolean {
  if (!capabilities || capabilities.length === 0) {
    return false
  }
  return capabilities.some(c => c.toLowerCase() === capability.toLowerCase())
}

/**
 * Check if a model supports multimodal (vision) capabilities based on Capabilities config
 * If capabilities is provided and contains "image", return true directly
 * Otherwise fallback to legacy detection based on provider and model ID
 */
export function supportsMultimodal(
  providerId: string,
  modelId: string,
  capabilities?: string[]
): boolean {
  // If capabilities is provided and contains "image", use it directly
  if (capabilities && hasCapability(capabilities, 'image')) {
    return true
  }

  // If capabilities is provided but does NOT contain "image", model does not support vision
  if (capabilities && capabilities.length > 0 && !hasCapability(capabilities, 'image')) {
    return false
  }

  // Fallback to legacy detection if capabilities is empty or not provided
  // (for backward compatibility with models that don't have capabilities set)

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
