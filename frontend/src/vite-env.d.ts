/// <reference types="vite/client" />

// Vue 单文件组件类型声明
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}

// Vite 环境变量类型
interface ImportMetaEnv {
  readonly VITE_APP_TITLE?: string
  // 添加更多环境变量类型...
}

interface ImportMeta {
  readonly env: ImportMetaEnv
  readonly hot?: {
    accept: () => void
    dispose: (cb: () => void) => void
  }
}

// SVG 作为 Vue 组件导入（配合 vite-svg-loader 的 defaultImport: 'component'）
declare module '*.svg' {
  import type { DefineComponent, SVGAttributes } from 'vue'
  const component: DefineComponent<SVGAttributes>
  export default component
}

// SVG 作为 URL 导入（使用 ?url 后缀）
declare module '*.svg?url' {
  const src: string
  export default src
}

// 图片资源
declare module '*.png' {
  const src: string
  export default src
}

declare module '*.jpg' {
  const src: string
  export default src
}

declare module '*.jpeg' {
  const src: string
  export default src
}

declare module '*.gif' {
  const src: string
  export default src
}

declare module '*.webp' {
  const src: string
  export default src
}

// CSS 模块
declare module '*.module.css' {
  const classes: { readonly [key: string]: string }
  export default classes
}

declare module '*.module.scss' {
  const classes: { readonly [key: string]: string }
  export default classes
}

// JSON
declare module '*.json' {
  const value: unknown
  export default value
}
