<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Dialogs } from '@wailsio/runtime'
import { Search, Upload, Plus } from 'lucide-vue-next'
import IconUploadFile from '@/assets/icons/upload-file.svg'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import DocumentCard from './DocumentCard.vue'
import type { Document, DocumentStatus } from './DocumentCard.vue'
import type { Library } from '@bindings/willchat/internal/services/library'

const props = defineProps<{
  library: Library
}>()

const { t } = useI18n()

const searchQuery = ref('')
const deleteDialogOpen = ref(false)
const documentToDelete = ref<Document | null>(null)

// Mock data for demonstration - will be replaced with actual API calls
const documents = ref<Document[]>([
  {
    id: 1,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 2,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 3,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 4,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 5,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 6,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'learning' as DocumentStatus,
    progress: 32,
  },
  {
    id: 7,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'failed' as DocumentStatus,
  },
  {
    id: 8,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 9,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 10,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 11,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
  {
    id: 12,
    name: '在核污染水处理问题上，日方应正视国际社会合理关切',
    fileType: 'pdf',
    createdAt: '2025-08-24',
    status: 'completed' as DocumentStatus,
  },
])

const filteredDocuments = computed(() => {
  if (!searchQuery.value.trim()) {
    return documents.value
  }
  const query = searchQuery.value.toLowerCase()
  return documents.value.filter((doc) => doc.name.toLowerCase().includes(query))
})

const handleAddDocument = async () => {
  try {
    const result = await Dialogs.OpenFile({
      Title: t('knowledge.content.selectFile'),
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: true,
      Filters: [
        {
          DisplayName: t('knowledge.content.fileTypes.documents'),
          Pattern: '*.pdf;*.doc;*.docx;*.txt;*.md',
        },
        {
          DisplayName: t('knowledge.content.fileTypes.all'),
          Pattern: '*.*',
        },
      ],
    })
    if (result && result.length > 0) {
      // TODO: Handle file upload with the selected paths
      console.log('Selected files:', result)
    }
  } catch (error) {
    console.error('Failed to open file dialog:', error)
  }
}

const handleRename = (doc: Document) => {
  // TODO: Implement rename dialog
  console.log('Rename document:', doc)
}

const handleOpenDelete = (doc: Document) => {
  documentToDelete.value = doc
  deleteDialogOpen.value = true
}

const confirmDelete = async () => {
  if (!documentToDelete.value) return
  // TODO: Call API to delete document
  documents.value = documents.value.filter((d) => d.id !== documentToDelete.value?.id)
  deleteDialogOpen.value = false
  documentToDelete.value = null
}
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- 头部区域 -->
    <div class="flex h-12 items-center justify-between px-4">
      <h2 class="text-base font-medium text-foreground">{{ library.name }}</h2>
      <div class="flex items-center gap-1.5">
        <!-- 搜索框 -->
        <div class="relative w-40">
          <Search class="absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground/50" />
          <Input
            v-model="searchQuery"
            type="text"
            :placeholder="t('knowledge.content.searchPlaceholder')"
            class="h-6 pr-7 text-xs placeholder:text-muted-foreground/40"
          />
        </div>
        <!-- 添加文档按钮 -->
        <Button
          variant="ghost"
          size="icon"
          class="size-6"
          :title="t('knowledge.content.addDocument')"
          @click="handleAddDocument"
        >
          <IconUploadFile class="size-4 text-muted-foreground" />
        </Button>
      </div>
    </div>

    <!-- 内容区域 -->
    <div class="flex-1 overflow-auto p-4">
      <!-- 空状态 -->
      <div
        v-if="filteredDocuments.length === 0"
        class="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground"
      >
        <Upload class="size-10 opacity-40" />
        <div class="text-center">
          <p class="text-sm">{{ t('knowledge.content.empty.title') }}</p>
          <p class="mt-1 text-xs text-muted-foreground/70">{{ t('knowledge.content.empty.desc') }}</p>
        </div>
        <Button variant="outline" size="sm" class="gap-1.5" @click="handleAddDocument">
          <Plus class="size-4" />
          {{ t('knowledge.content.addDocument') }}
        </Button>
      </div>

      <!-- 文档网格 -->
      <div
        v-else
        class="grid auto-rows-max gap-4"
        style="grid-template-columns: repeat(auto-fill, minmax(166px, 1fr))"
      >
        <DocumentCard
          v-for="doc in filteredDocuments"
          :key="doc.id"
          :document="doc"
          @rename="handleRename"
          @delete="handleOpenDelete"
        />
      </div>
    </div>

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('knowledge.content.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('knowledge.content.delete.desc', { name: documentToDelete?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            {{ t('knowledge.content.delete.cancel') }}
          </AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click.prevent="confirmDelete"
          >
            {{ t('knowledge.content.delete.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
