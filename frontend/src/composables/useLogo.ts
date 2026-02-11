import logoSvgRaw from '@/assets/images/logo.svg?raw'

/**
 * Convert logo SVG to a data URL.
 * The SVG uses `currentColor`; when rendered as an <img> data URL,
 * we inject a concrete color based on the current theme.
 */
export const getLogoDataUrl = () => {
  const isDark = document.documentElement.classList.contains('dark')
  // Use foreground-appropriate color: light text for dark mode, dark text for light mode
  const color = isDark ? '%23e5e5e5' : '%23171717'
  const svg = logoSvgRaw.replace(/currentColor/g, decodeURIComponent(color))
  return `data:image/svg+xml,${encodeURIComponent(svg)}`
}
