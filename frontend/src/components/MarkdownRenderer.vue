<script setup lang="ts">
import { computed, ref } from 'vue'
import { marked, type Tokens } from 'marked'
import hljs from 'highlight.js/lib/core'
import DOMPurify from 'dompurify'

// Import only commonly used languages to reduce bundle size
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import python from 'highlight.js/lib/languages/python'
import java from 'highlight.js/lib/languages/java'
import go from 'highlight.js/lib/languages/go'
import rust from 'highlight.js/lib/languages/rust'
import cpp from 'highlight.js/lib/languages/cpp'
import csharp from 'highlight.js/lib/languages/csharp'
import php from 'highlight.js/lib/languages/php'
import ruby from 'highlight.js/lib/languages/ruby'
import swift from 'highlight.js/lib/languages/swift'
import kotlin from 'highlight.js/lib/languages/kotlin'
import sql from 'highlight.js/lib/languages/sql'
import bash from 'highlight.js/lib/languages/bash'
import shell from 'highlight.js/lib/languages/shell'
import json from 'highlight.js/lib/languages/json'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import scss from 'highlight.js/lib/languages/scss'
import markdown from 'highlight.js/lib/languages/markdown'
import yaml from 'highlight.js/lib/languages/yaml'
import dockerfile from 'highlight.js/lib/languages/dockerfile'

// Register languages
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('java', java)
hljs.registerLanguage('go', go)
hljs.registerLanguage('rust', rust)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('c++', cpp)
hljs.registerLanguage('csharp', csharp)
hljs.registerLanguage('cs', csharp)
hljs.registerLanguage('php', php)
hljs.registerLanguage('ruby', ruby)
hljs.registerLanguage('rb', ruby)
hljs.registerLanguage('swift', swift)
hljs.registerLanguage('kotlin', kotlin)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('sh', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('json', json)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('css', css)
hljs.registerLanguage('scss', scss)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('md', markdown)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)
hljs.registerLanguage('dockerfile', dockerfile)

// Import highlight.js styles (using a dark theme that works well with both modes)
import 'highlight.js/styles/github-dark.css'

const props = defineProps<{
  content: string
  allowHtml?: boolean
  isStreaming?: boolean // Show blinking cursor at end of content
}>()

// Track copied state for each code block
const copiedBlocks = ref<Set<number>>(new Set())

let blockIndex = 0

// Custom renderer using new marked v13+ API
const renderer: Partial<import('marked').RendererObject> = {
  code({ text, lang }: Tokens.Code) {
    const language = lang || 'plaintext'
    const validLang = hljs.getLanguage(language) ? language : 'plaintext'
    const highlighted = hljs.highlight(text, { language: validLang }).value
    const currentIndex = blockIndex++

    return `
      <div class="code-block-wrapper group relative my-3 overflow-hidden rounded-lg border border-border bg-zinc-950">
        <div class="flex items-center justify-between border-b border-border/50 bg-zinc-900 px-4 py-2">
          <span class="text-xs text-zinc-400">${language}</span>
          <button
            class="copy-btn flex items-center gap-1.5 rounded px-2 py-1 text-xs text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
            data-code="${encodeURIComponent(text)}"
            data-index="${currentIndex}"
          >
            <svg class="copy-icon size-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="14" height="14" x="8" y="8" rx="2" ry="2"/>
              <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>
            </svg>
            <span class="copy-text">Copy</span>
          </button>
        </div>
        <pre class="overflow-x-auto p-4"><code class="hljs language-${validLang}">${highlighted}</code></pre>
      </div>
    `
  },

  codespan({ text }: Tokens.Codespan) {
    return `<code class="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">${text}</code>`
  },

  link({ href, title, text }: Tokens.Link) {
    const titleAttr = title ? ` title="${title}"` : ''
    return `<a href="${href}"${titleAttr} class="text-primary underline hover:text-primary/80" target="_blank" rel="noopener noreferrer">${text}</a>`
  },

  table({ header, rows }: Tokens.Table) {
    const headerHtml = header
      .map((cell) => {
        const align = cell.align ? ` style="text-align: ${cell.align}"` : ''
        return `<th class="px-4 py-2 font-medium"${align}>${this.parser?.parseInline(cell.tokens) || cell.text}</th>`
      })
      .join('')

    const bodyHtml = rows
      .map((row) => {
        const cells = row
          .map((cell) => {
            const align = cell.align ? ` style="text-align: ${cell.align}"` : ''
            return `<td class="px-4 py-2"${align}>${this.parser?.parseInline(cell.tokens) || cell.text}</td>`
          })
          .join('')
        return `<tr class="border-b border-border">${cells}</tr>`
      })
      .join('')

    return `
      <div class="my-3 overflow-x-auto">
        <table class="min-w-full border-collapse border border-border text-sm">
          <thead class="bg-muted/50"><tr class="border-b border-border">${headerHtml}</tr></thead>
          <tbody>${bodyHtml}</tbody>
        </table>
      </div>
    `
  },

  blockquote({ tokens }: Tokens.Blockquote) {
    const body = this.parser?.parse(tokens) || ''
    return `<blockquote class="my-3 border-l-4 border-primary/50 pl-4 italic text-muted-foreground">${body}</blockquote>`
  },

  list({ items, ordered }: Tokens.List) {
    const tag = ordered ? 'ol' : 'ul'
    const classes = ordered ? 'list-decimal' : 'list-disc'
    const body = items
      .map((item) => {
        const content = this.parser?.parse(item.tokens) || item.text
        return `<li class="pl-1">${content}</li>`
      })
      .join('')
    return `<${tag} class="my-3 ml-6 space-y-1 ${classes}">${body}</${tag}>`
  },

  heading({ tokens, depth }: Tokens.Heading) {
    const text = this.parser?.parseInline(tokens) || ''
    const sizes: Record<number, string> = {
      1: 'text-2xl font-bold mt-6 mb-3',
      2: 'text-xl font-semibold mt-5 mb-2',
      3: 'text-lg font-semibold mt-4 mb-2',
      4: 'text-base font-semibold mt-3 mb-1',
      5: 'text-sm font-semibold mt-2 mb-1',
      6: 'text-sm font-medium mt-2 mb-1',
    }
    return `<h${depth} class="${sizes[depth] || ''}">${text}</h${depth}>`
  },

  paragraph({ tokens }: Tokens.Paragraph) {
    const text = this.parser?.parseInline(tokens) || ''
    return `<p class="my-2 leading-relaxed">${text}</p>`
  },

  hr() {
    return `<hr class="my-4 border-border" />`
  },

  strong({ tokens }: Tokens.Strong) {
    const text = this.parser?.parseInline(tokens) || ''
    return `<strong class="font-semibold">${text}</strong>`
  },

  em({ tokens }: Tokens.Em) {
    const text = this.parser?.parseInline(tokens) || ''
    return `<em class="italic">${text}</em>`
  },
}

// Configure marked
marked.use({
  gfm: true,
  breaks: true,
  renderer,
})

const renderedHtml = computed(() => {
  // Reset block index for each render
  blockIndex = 0

  const rawHtml = marked.parse(props.content) as string

  // Sanitize HTML to prevent XSS
  let clean = DOMPurify.sanitize(rawHtml, {
    ADD_ATTR: ['target', 'data-code', 'data-index'],
    ADD_TAGS: ['button'],
  })

  // Inject blinking cursor inline at the end of the last block element when streaming
  if (props.isStreaming) {
    const cursorHtml = '<span class="streaming-cursor" aria-hidden="true"></span>'
    // Insert cursor before the last closing block-level tag to keep it inline
    const lastBlockClose = /(<\/(?:p|li|h[1-6]|td|blockquote|span|em|strong|code|a)>)(\s*)$/i
    if (lastBlockClose.test(clean)) {
      clean = clean.replace(lastBlockClose, cursorHtml + '$1$2')
    } else {
      // Fallback: append at the end (e.g. empty content or non-standard structure)
      clean += cursorHtml
    }
  }

  return clean
})

// Handle copy button clicks
const handleClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  const copyBtn = target.closest('.copy-btn') as HTMLElement

  if (copyBtn) {
    const code = decodeURIComponent(copyBtn.dataset.code || '')
    const index = parseInt(copyBtn.dataset.index || '0', 10)

    navigator.clipboard.writeText(code).then(() => {
      copiedBlocks.value.add(index)
      const textEl = copyBtn.querySelector('.copy-text')
      const iconEl = copyBtn.querySelector('.copy-icon')

      if (textEl) textEl.textContent = 'Copied!'
      if (iconEl) {
        iconEl.innerHTML = `
          <polyline points="20 6 9 17 4 12"/>
        `
      }

      setTimeout(() => {
        copiedBlocks.value.delete(index)
        if (textEl) textEl.textContent = 'Copy'
        if (iconEl) {
          iconEl.innerHTML = `
            <rect width="14" height="14" x="8" y="8" rx="2" ry="2"/>
            <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>
          `
        }
      }, 2000)
    })
  }
}
</script>

<template>
  <div
    class="markdown-content prose prose-sm dark:prose-invert min-w-0 w-full max-w-full"
    @click="handleClick"
    v-html="renderedHtml"
  />
</template>

<style>
/* Additional markdown styles */
.markdown-content {
  /* Match user message font size (text-sm = 0.875rem = 14px) */
  font-size: 0.875rem;
  line-height: 1.5rem;
  /* Ensure proper text wrapping and prevent overflow */
  word-wrap: break-word;
  overflow-wrap: break-word;
  word-break: break-word;
  /* Ensure content doesn't cause horizontal overflow */
  max-width: 100%;
}

.markdown-content pre {
  margin: 0;
  background: transparent;
}

.markdown-content pre code {
  font-size: 0.8125rem;
  line-height: 1.5;
}

.markdown-content code.hljs {
  padding: 0;
  background: transparent;
}

/* Dark mode adjustments for inline code */
.dark .markdown-content code:not(.hljs) {
  background-color: hsl(var(--muted));
}

/* Ensure code blocks don't overflow */
.code-block-wrapper {
  max-width: 100%;
}

.code-block-wrapper pre {
  max-width: 100%;
}

/* Streaming blinking cursor (injected via v-html) */
.streaming-cursor {
  display: inline-block;
  width: 2px;
  height: 1.1em;
  margin-left: 1px;
  vertical-align: text-bottom;
  background-color: currentColor;
  opacity: 0.7;
  animation: streaming-cursor-blink 1s steps(2, start) infinite;
}

@keyframes streaming-cursor-blink {
  to {
    opacity: 0;
  }
}
</style>
