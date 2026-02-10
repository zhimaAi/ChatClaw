import logoSvgRaw from '@/assets/images/logo.svg?raw'

/**
 * 将 logo SVG 转换为 data URL
 * Logo 使用品牌蓝色 (#3F8DFF)，无需根据主题切换颜色
 */
export const getLogoDataUrl = () => {
  return `data:image/svg+xml,${encodeURIComponent(logoSvgRaw)}`
}
