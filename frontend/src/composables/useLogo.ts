import logoSvgRaw from '@/assets/images/logo.svg?raw'

/**
 * 将 logo SVG 转换为 data URL
 * 替换 currentColor 为具体颜色以确保在 img 标签中正常显示
 * @param isDark 是否深色模式
 */
export const getLogoDataUrl = (isDark?: boolean) => {
  // 如果没有传入参数，自动检测当前主题
  const dark = isDark ?? document.documentElement.classList.contains('dark')
  // 深色模式用浅色图标，浅色模式用深色图标
  const color = dark ? '#e5e5e5' : '#404040'
  const svgWithColor = logoSvgRaw.replace(/currentColor/g, color)
  return `data:image/svg+xml,${encodeURIComponent(svgWithColor)}`
}
