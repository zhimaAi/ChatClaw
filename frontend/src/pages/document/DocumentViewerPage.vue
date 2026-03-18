<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useNavigationStore, type DocumentViewerData } from '@/stores/navigation'
import { ExternalLink } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { DocumentService } from '@bindings/chatclaw/internal/services/document'
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'
// Import vue-office components
import VueOfficeDocx from '@vue-office/docx'
import VueOfficeExcel from '@vue-office/excel'
import '@vue-office/docx/lib/index.css'
import '@vue-office/excel/lib/index.css'
// Import bestofdview for OFD preview
import { OfdView } from 'bestofdview'
import 'bestofdview/dist/style.css'

const props = defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()

const documentId = ref<number | null>(null)
const documentName = ref<string>('')
const fileType = ref<string>('')
const filePath = ref<string>('')
const fileContent = ref<string>('')
const fileBuffer = ref<ArrayBuffer | null>(null)
// For PDF/HTML and other binary types rendered via iframe using data URLs or blob URLs
const fileDataUrl = ref<string>('')
// Blob URL for PDF/OFD files (to avoid URL length limits and file:// restrictions)
const blobUrl = ref<string>('')
const loading = ref(false)
const error = ref<string>('')
const renderError = ref<string>('') // Error during rendering (e.g., corrupted file, wrong type)
const viewerType = ref<
  'iframe' | 'text' | 'html' | 'markdown' | 'csv' | 'docx' | 'xlsx' | 'ofd' | 'unsupported'
>('unsupported')

/**
 * Checks whether the given buffer starts with a valid ZIP local-file header
 * (PK signature with accepted version bytes).
 */
const isValidZipHeader = (buf: ArrayBuffer): boolean => {
  if (buf.byteLength < 4) return false
  const h = new Uint8Array(buf, 0, 4)
  return (
    h[0] === 0x50 &&
    h[1] === 0x4b &&
    (h[2] === 0x03 || h[2] === 0x05 || h[2] === 0x07) &&
    (h[3] === 0x04 || h[3] === 0x06 || h[3] === 0x08)
  )
}

const isValidPdfHeader = (buf: ArrayBuffer): boolean => {
  if (buf.byteLength < 5) return false
  const h = new Uint8Array(buf, 0, 5)
  return String.fromCharCode(...h).startsWith('%PDF')
}

// Get document data from tab
const currentTab = computed(() => {
  return navigationStore.tabs.find((tab) => tab.id === props.tabId)
})

const documentData = computed<DocumentViewerData | undefined>(() => {
  return currentTab.value?.data as DocumentViewerData | undefined
})

// Determine viewer type based on file extension
const determineViewerType = (
  fileType: string,
  path: string
): 'iframe' | 'text' | 'html' | 'markdown' | 'csv' | 'docx' | 'xlsx' | 'ofd' | 'unsupported' => {
  const ext = fileType.toLowerCase()

  // OFD files cannot be previewed in browser, use external app
  if (ext === 'ofd') {
    return 'ofd'
  }

  // PDF can be viewed in iframe (streaming support)
  if (ext === 'pdf') {
    return 'iframe'
  }

  // HTML files (streaming support via iframe)
  if (ext === 'html' || ext === 'htm') {
    return 'html'
  }

  // Markdown files
  if (ext === 'md' || ext === 'markdown') {
    return 'markdown'
  }

  // CSV files
  if (ext === 'csv') {
    return 'csv'
  }

  // Text files
  if (ext === 'txt') {
    return 'text'
  }

  // DOCX files - use vue-office
  if (ext === 'docx' || ext === 'doc') {
    return 'docx'
  }

  // XLSX files - use vue-office
  if (ext === 'xlsx' || ext === 'xls') {
    return 'xlsx'
  }

  return 'unsupported'
}

// Load document information and content
const loadDocument = async () => {
  const data = documentData.value
  if (!data || !data.documentId) {
    error.value = t('knowledge.viewer.notFound')
    return
  }

  documentId.value = data.documentId
  documentName.value = data.documentName

  loading.value = true
  error.value = ''
  renderError.value = ''
  filePath.value = ''
  fileContent.value = ''
  fileBuffer.value = null
  fileDataUrl.value = ''
  // Clean up previous blob URL if exists
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = ''
  }

  try {
    const path = await DocumentService.GetDocumentPath(data.documentId)
    filePath.value = path

    // Get file extension from path
    const ext = path.split('.').pop()?.toLowerCase() || ''
    fileType.value = ext
    viewerType.value = determineViewerType(ext, path)

    // For HTML files, use data URL to avoid file:// restrictions
    if (
      viewerType.value === 'html' &&
      !(path.startsWith('http://') || path.startsWith('https://'))
    ) {
      try {
        // @ts-ignore - GetDocumentBytes may not be in type definitions yet
        const base64Content = await DocumentService.GetDocumentBytes(data.documentId)
        if (base64Content) {
          const mime = 'text/html; charset=utf-8'
          fileDataUrl.value = `data:${mime};base64,${base64Content}`
        }
      } catch (err) {
        console.warn(
          'Failed to load document bytes for html preview, falling back to file path:',
          err
        )
        // Fallback to original file path based preview (may be limited by browser security policies)
      } finally {
        loading.value = false
      }
      // For HTML we are done here
      return
    }

    // For OFD files, use Blob URL for bestofdview
    if (viewerType.value === 'ofd') {
      try {
        // @ts-ignore - GetDocumentBytes may not be in type definitions yet
        const base64Content = await DocumentService.GetDocumentBytes(data.documentId)
        if (base64Content) {
          // Convert base64 to ArrayBuffer
          const binaryString = atob(base64Content)
          const bytes = new Uint8Array(binaryString.length)
          for (let i = 0; i < binaryString.length; i++) {
            bytes[i] = binaryString.charCodeAt(i)
          }

          if (!isValidZipHeader(bytes.buffer)) {
            renderError.value = t('knowledge.viewer.loadFailedUseExternal')
            loading.value = false
            return
          }

          // Create Blob and generate blob URL for OFD
          const blob = new Blob([bytes.buffer], { type: 'application/ofd' })
          // Clean up previous blob URL if exists
          if (blobUrl.value) {
            URL.revokeObjectURL(blobUrl.value)
          }
          blobUrl.value = URL.createObjectURL(blob)
        } else {
          // No content available
          renderError.value = t('knowledge.viewer.loadFailedUseExternal')
          loading.value = false
          return
        }
      } catch (err) {
        console.warn('Failed to load document bytes for OFD preview:', err)
        renderError.value = t('knowledge.viewer.loadFailedUseExternal')
        loading.value = false
        return
      } finally {
        loading.value = false
      }
      return
    }

    // For PDF files, use Blob URL to avoid file:// restrictions and URL length limits
    if (viewerType.value === 'iframe') {
      try {
        // @ts-ignore - GetDocumentBytes may not be in type definitions yet
        const base64Content = await DocumentService.GetDocumentBytes(data.documentId)
        if (base64Content) {
          // Convert base64 to ArrayBuffer
          const binaryString = atob(base64Content)
          const bytes = new Uint8Array(binaryString.length)
          for (let i = 0; i < binaryString.length; i++) {
            bytes[i] = binaryString.charCodeAt(i)
          }

          if (!isValidPdfHeader(bytes.buffer)) {
            renderError.value = t('knowledge.viewer.corruptedOrWrongType', { type: 'PDF' })
            return
          }

          // Create Blob and generate blob URL
          const blob = new Blob([bytes.buffer], { type: 'application/pdf' })
          // Clean up previous blob URL if exists
          if (blobUrl.value) {
            URL.revokeObjectURL(blobUrl.value)
          }
          blobUrl.value = URL.createObjectURL(blob)
        } else {
          renderError.value = t('knowledge.viewer.loadFailedUseExternal')
        }
      } catch (err) {
        console.warn('Failed to load document bytes for PDF preview:', err)
        renderError.value =
          getErrorMessage(err) || t('knowledge.viewer.corruptedOrWrongType', { type: 'PDF' })
      } finally {
        loading.value = false
      }
      return
    }

    // For Office files (docx, xlsx), load as ArrayBuffer for vue-office
    if (viewerType.value === 'docx' || viewerType.value === 'xlsx') {
      try {
        if (path.startsWith('http://') || path.startsWith('https://')) {
          // For web URLs, fetch directly
          const response = await fetch(path)
          if (response.ok) {
            fileBuffer.value = await response.arrayBuffer()
          } else {
            throw new Error('Failed to load file content')
          }
        } else {
          // For local files, use backend API to get file bytes (base64 encoded)
          try {
            // @ts-ignore - GetDocumentBytes may not be in type definitions yet
            const base64Content = await DocumentService.GetDocumentBytes(data.documentId)
            // Convert base64 to ArrayBuffer
            const binaryString = atob(base64Content)
            const bytes = new Uint8Array(binaryString.length)
            for (let i = 0; i < binaryString.length; i++) {
              bytes[i] = binaryString.charCodeAt(i)
            }
            fileBuffer.value = bytes.buffer
          } catch (apiErr: any) {
            // If GetDocumentBytes is not available, try file:// URL fallback
            if (
              apiErr?.message?.includes('not a function') ||
              apiErr?.message?.includes('undefined')
            ) {
              console.warn('GetDocumentBytes not available, trying file:// URL fallback')
              // Fallback: try to fetch via file:// URL (may have CORS restrictions)
              let normalizedPath = path.replace(/\\/g, '/')
              if (!normalizedPath.startsWith('/')) {
                normalizedPath = '/' + normalizedPath
              }
              if (normalizedPath.match(/^\/[A-Z]:\//i)) {
                normalizedPath = normalizedPath.substring(1)
              }
              const localFileUrl = `file:///${normalizedPath}`

              const response = await fetch(localFileUrl)
              if (response.ok) {
                fileBuffer.value = await response.arrayBuffer()
              } else {
                throw new Error('Failed to load file via file:// URL')
              }
            } else {
              throw apiErr
            }
          }
        }

        if (fileBuffer.value) {
          if (!isValidZipHeader(fileBuffer.value)) {
            renderError.value = t('knowledge.viewer.corruptedOrWrongType', {
              type: viewerType.value.toUpperCase(),
            })
          }
        } else {
          renderError.value = t('knowledge.viewer.loadFailedUseExternal')
        }
      } catch (err) {
        console.error('Failed to load Office file:', err)
        renderError.value =
          getErrorMessage(err) ||
          t('knowledge.viewer.corruptedOrWrongType', { type: viewerType.value.toUpperCase() })
      }
    }
    // For text-based files, try to load content from backend
    else if (
      viewerType.value === 'text' ||
      viewerType.value === 'markdown' ||
      viewerType.value === 'csv'
    ) {
      try {
        if (path.startsWith('http://') || path.startsWith('https://')) {
          // For web URLs, fetch directly
          const response = await fetch(path)
          if (response.ok) {
            fileContent.value = await response.text()
          } else {
            throw new Error('Failed to load file content')
          }
        } else {
          // For local files, use backend API to read content
          try {
            fileContent.value = await DocumentService.GetDocumentContent(data.documentId)
          } catch (err) {
            // If content reading fails, show file path option
            console.warn('Failed to load file content from backend:', err)
            // Don't set error, just leave fileContent empty - UI will show fallback
          }
        }
      } catch (err) {
        console.error('Failed to load file content:', err)
        // Don't set error for text files - show fallback UI instead
      }
    }
  } catch (err) {
    console.error('Failed to get document path:', err)
    error.value = getErrorMessage(err) || t('knowledge.viewer.loadFailed')
    viewerType.value = 'unsupported'
  } finally {
    loading.value = false
  }
}

// Watch for tab data changes
watch(
  () => documentData.value,
  async (data) => {
    if (data && data.documentId) {
      await loadDocument()
    }
  },
  { immediate: true }
)

// Generate file URL for iframe
const fileUrl = computed(() => {
  // Prefer blob URL for PDF/OFD (avoids file:// restrictions and URL length limits)
  if (blobUrl.value) return blobUrl.value
  // Prefer data URL when available (for HTML files)
  if (fileDataUrl.value) return fileDataUrl.value
  if (!filePath.value) return ''

  // For web URLs, use as-is
  if (filePath.value.startsWith('http://') || filePath.value.startsWith('https://')) {
    return filePath.value
  }

  // For local files, we should have loaded them as blob URLs by now
  // This fallback should rarely be used
  return ''
})

// Clean up blob URL on component unmount
onUnmounted(() => {
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = ''
  }
})

// Parse CSV content
const csvData = computed(() => {
  if (!fileContent.value || viewerType.value !== 'csv') return []

  const lines = fileContent.value.split('\n').filter((line) => line.trim())
  if (lines.length === 0) return []

  // Try to detect delimiter
  const firstLine = lines[0]
  const delimiter = firstLine.includes(',') ? ',' : firstLine.includes('\t') ? '\t' : ','

  return lines.map((line) => {
    // Simple CSV parsing (doesn't handle quoted fields with commas)
    return line.split(delimiter).map((cell) => cell.trim())
  })
})

// Handle open externally
const handleOpenExternally = async () => {
  if (!documentId.value) return

  try {
    await DocumentService.OpenDocument(documentId.value)
  } catch (err) {
    console.error('Failed to open document:', err)
    toast.error(getErrorMessage(err) || t('knowledge.viewer.openFailed'))
  }
}

onMounted(() => {
  void loadDocument()
})
</script>

<template>
  <div class="flex h-full w-full flex-col bg-background">
    <!-- Header -->
    <div class="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
      <div class="flex items-center gap-2">
        <h1 class="truncate text-base font-medium text-foreground">
          {{ documentName || t('knowledge.viewer.title') }}
        </h1>
      </div>
      <div class="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          class="gap-2"
          :title="t('knowledge.viewer.openExternally')"
          @click="handleOpenExternally"
        >
          <ExternalLink class="size-4" />
          {{ t('knowledge.viewer.openExternally') }}
        </Button>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-hidden">
      <!-- Loading state -->
      <div v-if="loading" class="flex h-full flex-col items-center justify-center gap-4">
        <!-- Spinner animation -->
        <div class="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
        <!-- Loading text -->
        <div class="flex flex-col items-center gap-2">
          <div class="text-sm font-medium text-foreground">{{ t('knowledge.loading') }}</div>
          <div
            v-if="viewerType === 'ofd' || viewerType === 'iframe'"
            class="text-xs text-muted-foreground"
          >
            {{
              viewerType === 'ofd'
                ? t('knowledge.viewer.loadingOfd')
                : t('knowledge.viewer.loadingPdf')
            }}
          </div>
        </div>
        <!-- Progress bar (indeterminate) -->
        <div v-if="viewerType === 'ofd' || viewerType === 'iframe'" class="w-64">
          <div class="h-1 w-full overflow-hidden rounded bg-muted">
            <div
              class="h-full w-1/3 animate-[progress_1.5s_ease-in-out_infinite] bg-foreground/30"
            />
          </div>
        </div>
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="flex h-full flex-col items-center justify-center gap-4">
        <div class="text-sm text-muted-foreground text-center">{{ error }}</div>
        <Button variant="outline" class="gap-2" @click="handleOpenExternally">
          <ExternalLink class="size-4 shrink-0" />
          {{ t('knowledge.viewer.openExternally') }}
        </Button>
      </div>

      <!-- Unsupported file type -->
      <div
        v-else-if="viewerType === 'unsupported'"
        class="flex h-full flex-col items-center justify-center gap-4"
      >
        <div class="text-sm text-muted-foreground text-center">
          {{ t('knowledge.viewer.unsupported', { type: fileType.toUpperCase() }) }}
        </div>
        <Button variant="outline" class="gap-2" @click="handleOpenExternally">
          <ExternalLink class="size-4 shrink-0" />
          {{ t('knowledge.viewer.openExternally') }}
        </Button>
      </div>

      <!-- DOCX viewer (vue-office) -->
      <div v-else-if="viewerType === 'docx'" class="h-full overflow-auto">
        <div v-if="renderError" class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ renderError }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
        <VueOfficeDocx
          v-else-if="fileBuffer"
          :src="fileBuffer"
          style="height: 100%"
          @rendered="renderError = ''"
          @error="
            (err: any) => {
              console.error('DOCX render error:', err)
              renderError = t('knowledge.viewer.corruptedOrWrongType', { type: 'DOCX' })
            }
          "
        />
        <div v-else class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>

      <!-- XLSX viewer (vue-office) -->
      <div v-else-if="viewerType === 'xlsx'" class="h-full overflow-auto">
        <div v-if="renderError" class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ renderError }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
        <VueOfficeExcel
          v-else-if="fileBuffer"
          :src="fileBuffer"
          style="height: 100%"
          @rendered="renderError = ''"
          @error="
            (err: any) => {
              console.error('XLSX render error:', err)
              renderError = t('knowledge.viewer.corruptedOrWrongType', { type: 'XLSX' })
            }
          "
        />
        <div v-else class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>

      <!-- PDF viewer (iframe - streaming) -->
      <div v-else-if="viewerType === 'iframe'" class="h-full overflow-hidden relative">
        <!-- Loading overlay for PDF -->
        <div
          v-if="loading || (!fileUrl && !error && !renderError)"
          class="absolute inset-0 flex flex-col items-center justify-center bg-background gap-4 z-10"
        >
          <div class="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
          <div class="flex flex-col items-center gap-2">
            <div class="text-sm font-medium text-foreground">{{ t('knowledge.loading') }}</div>
            <div class="text-xs text-muted-foreground">{{ t('knowledge.viewer.loadingPdf') }}</div>
          </div>
          <div class="w-64">
            <div class="h-1 w-full overflow-hidden rounded bg-muted">
              <div
                class="h-full w-1/3 animate-[progress_1.5s_ease-in-out_infinite] bg-foreground/30"
              />
            </div>
          </div>
        </div>
        <!-- Error overlay for PDF -->
        <div
          v-if="renderError"
          class="absolute inset-0 flex flex-col items-center justify-center bg-background gap-4 z-10"
        >
          <div class="text-sm text-muted-foreground text-center">
            {{ renderError }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
        <iframe
          v-if="fileUrl && !renderError"
          :src="fileUrl"
          class="h-full w-full border-0"
          :title="documentName"
          @error="
            () => {
              renderError = t('knowledge.viewer.corruptedOrWrongType', { type: 'PDF' })
            }
          "
        />
      </div>

      <!-- OFD viewer (using bestofdview) -->
      <div v-else-if="viewerType === 'ofd'" class="h-full overflow-hidden flex flex-col relative">
        <!-- Loading overlay for OFD -->
        <div
          v-if="!blobUrl && loading && !renderError"
          class="absolute inset-0 flex flex-col items-center justify-center bg-background gap-4 z-10"
        >
          <div class="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
          <div class="flex flex-col items-center gap-2">
            <div class="text-sm font-medium text-foreground">{{ t('knowledge.loading') }}</div>
            <div class="text-xs text-muted-foreground">{{ t('knowledge.viewer.loadingOfd') }}</div>
          </div>
          <div class="w-64">
            <div class="h-1 w-full overflow-hidden rounded bg-muted">
              <div
                class="h-full w-1/3 animate-[progress_1.5s_ease-in-out_infinite] bg-foreground/30"
              />
            </div>
          </div>
        </div>
        <!-- Error state for OFD -->
        <div
          v-if="renderError"
          class="absolute inset-0 flex flex-col items-center justify-center bg-background gap-4 z-10"
        >
          <div class="text-sm text-muted-foreground text-center">
            {{ renderError }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
        <div v-else-if="blobUrl" class="flex-1 overflow-hidden">
          <OfdView
            :show-open-file-button="false"
            :ofd-link="blobUrl"
            class="h-full w-full"
            @error="
              (err: any) => {
                console.error('OFD render error:', err)
                renderError = t('knowledge.viewer.loadFailedUseExternal')
                loading = false
              }
            "
          />
        </div>
        <div v-else-if="!loading" class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>

      <!-- HTML viewer (iframe - streaming) -->
      <div v-else-if="viewerType === 'html'" class="h-full overflow-hidden">
        <iframe :src="fileUrl" class="h-full w-full border-0" :title="documentName" />
      </div>

      <!-- Markdown viewer -->
      <div v-else-if="viewerType === 'markdown'" class="h-full overflow-auto p-6">
        <div v-if="fileContent" class="prose prose-sm dark:prose-invert max-w-none">
          <MarkdownRenderer :content="fileContent" />
        </div>
        <div v-else class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>

      <!-- CSV viewer -->
      <div v-else-if="viewerType === 'csv'" class="h-full overflow-auto p-6">
        <div v-if="csvData.length > 0" class="overflow-x-auto">
          <table class="w-full border-collapse border border-border text-sm">
            <thead>
              <tr>
                <th
                  v-for="(header, index) in csvData[0]"
                  :key="index"
                  class="border border-border bg-muted px-4 py-2 text-left font-medium"
                >
                  {{ header }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, rowIndex) in csvData.slice(1)" :key="rowIndex">
                <td
                  v-for="(cell, cellIndex) in row"
                  :key="cellIndex"
                  class="border border-border px-4 py-2"
                >
                  {{ cell }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>

      <!-- Text viewer -->
      <div v-else-if="viewerType === 'text'" class="h-full overflow-auto p-6">
        <pre
          v-if="fileContent"
          class="text-sm font-mono whitespace-pre-wrap break-words text-foreground"
          >{{ fileContent }}</pre
        >
        <div v-else class="flex h-full flex-col items-center justify-center gap-4">
          <div class="text-sm text-muted-foreground text-center">
            {{ t('knowledge.viewer.contentNotAvailable') }}
          </div>
          <Button variant="outline" class="gap-2" @click="handleOpenExternally">
            <ExternalLink class="size-4 shrink-0" />
            {{ t('knowledge.viewer.openExternally') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes progress {
  0% {
    transform: translateX(-100%);
  }
  50% {
    transform: translateX(300%);
  }
  100% {
    transform: translateX(-100%);
  }
}
</style>
