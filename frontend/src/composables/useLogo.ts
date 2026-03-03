import { ref } from 'vue'
import logoLight from '@/assets/images/logo-light.png'
import logoDark from '@/assets/images/logo-dark.png'

// Shared reactive logo src for the current window.
const logoSrcRef = ref<string>(logoLight)
let logoObserver: InstanceType<typeof window.MutationObserver> | null = null
let logoInitialized = false

function computeLogoByDom(): string {
  if (typeof document === 'undefined') return logoLight
  const isDark = document.documentElement.classList.contains('dark')
  return isDark ? logoDark : logoLight
}

function ensureLogoInitialized() {
  if (logoInitialized || typeof window === 'undefined') return
  logoInitialized = true

  // Set initial value based on current DOM theme.
  logoSrcRef.value = computeLogoByDom()

  // Observe html.dark class changes and update logo for this window.
  logoObserver = new window.MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.attributeName === 'class') {
        logoSrcRef.value = computeLogoByDom()
        break
      }
    }
  })

  logoObserver.observe(document.documentElement, { attributes: true })
}

/**
 * Get theme-aware logo URL.
 * - Light theme: use logo-light.png (darker lobster for light background).
 * - Dark theme: use logo-dark.png (brighter lobster for dark background).
 *
 * NOTE:
 * - This is primarily used for PNG-based logo usages that should react to theme changes.
 */
export const getLogoDataUrl = (): string => {
  ensureLogoInitialized()
  return logoSrcRef.value
}

/**
 * Convenience helper for components that want a theme-aware src binding.
 * Usage in Vue template:
 *   const { logoSrc } = useThemeLogo()
 *   <img :src="logoSrc" ... />
 */
export const useThemeLogo = () => {
  ensureLogoInitialized()
  return { logoSrc: logoSrcRef }
}
