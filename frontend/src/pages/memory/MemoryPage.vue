<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FileText, FolderOpen, Save, Plus, Trash2 } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { MemoryService } from '@bindings/chatclaw/internal/services/memory'
import type { MemoryFile } from '@bindings/chatclaw/internal/services/memory'
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

const { t } = useI18n()

/* ── agent list ── */
const agents = ref<Agent[]>([])
const selectedAgentId = ref<number | null>(null)

const selectedAgent = computed(() =>
  agents.value.find((a) => a.id === selectedAgentId.value) ?? null
)

// Get openclaw_workspace_id from the selected agent
const workspaceId = computed(() => {
  const agent = selectedAgent.value
  if (!agent) return ''
  return (agent as any).openclaw_agent_id ?? ''
})

async function loadAgents() {
  try {
    agents.value = await AgentsService.ListAgents()
    if (agents.value.length && !selectedAgentId.value) {
      selectedAgentId.value = agents.value[0].id
    }
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

/* ── memory files ── */
const memoryFiles = ref<MemoryFile[]>([])
const selectedFilePath = ref<string | null>(null)
const fileContent = ref('')
const originalContent = ref('')
const isEditing = ref(false)
const isLoading = ref(false)

const hasChanges = computed(() => fileContent.value !== originalContent.value)

const selectedFile = computed(() =>
  memoryFiles.value.find((f) => f.path === selectedFilePath.value) ?? null
)

// Whether the agent has OpenClaw workspace configured
const hasWorkspace = computed(() => workspaceId.value !== '')

async function loadFiles() {
  if (!workspaceId.value) {
    memoryFiles.value = []
    selectedFilePath.value = null
    return
  }
  isLoading.value = true
  try {
    memoryFiles.value = await MemoryService.ListMemoryFiles(workspaceId.value)
    // Auto-select first file if none selected
    if (memoryFiles.value.length && !selectedFilePath.value) {
      await selectFile(memoryFiles.value[0].path)
    }
  } catch (e) {
    memoryFiles.value = []
    // Don't show error toast for missing workspace — it's expected
  } finally {
    isLoading.value = false
  }
}

async function selectFile(path: string) {
  if (hasChanges.value && !confirm(t('memory.unsavedChangesConfirm'))) {
    return
  }
  selectedFilePath.value = path
  isEditing.value = false
  try {
    const content = await MemoryService.ReadMemoryFile(workspaceId.value, path)
    fileContent.value = content
    originalContent.value = content
  } catch (e) {
    fileContent.value = ''
    originalContent.value = ''
    toast.error(getErrorMessage(e))
  }
}

async function saveFile() {
  if (!selectedFilePath.value) return
  try {
    await MemoryService.WriteMemoryFile(workspaceId.value, selectedFilePath.value, fileContent.value)
    originalContent.value = fileContent.value
    isEditing.value = false
    toast.success(t('memory.saveSuccess'))
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

function cancelEdit() {
  fileContent.value = originalContent.value
  isEditing.value = false
}

/* ── create new file ── */
const showNewFileDialog = ref(false)
const newFileName = ref('')

async function createNewFile() {
  let name = newFileName.value.trim()
  if (!name) return
  if (!name.endsWith('.md')) name += '.md'

  try {
    await MemoryService.WriteMemoryFile(workspaceId.value, name, '')
    showNewFileDialog.value = false
    newFileName.value = ''
    await loadFiles()
    await selectFile(name)
    isEditing.value = true
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

/* ── delete file ── */
const showDeleteDialog = ref(false)

async function deleteFile() {
  if (!selectedFilePath.value) return
  try {
    await MemoryService.DeleteMemoryFile(workspaceId.value, selectedFilePath.value)
    showDeleteDialog.value = false
    selectedFilePath.value = null
    fileContent.value = ''
    originalContent.value = ''
    toast.success(t('memory.deleteSuccess'))
    await loadFiles()
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

/* ── category labels ── */
function categoryLabel(cat: string): string {
  switch (cat) {
    case 'core': return t('memory.categoryCore')
    case 'persona': return t('memory.categoryPersona')
    case 'daily': return t('memory.categoryDaily')
    default: return t('memory.categoryOther')
  }
}

function categoryIcon(cat: string): string {
  switch (cat) {
    case 'core': return '📝'
    case 'persona': return '🧠'
    case 'daily': return '📅'
    default: return '📄'
  }
}

/* ── lifecycle ── */
watch(selectedAgentId, () => {
  selectedFilePath.value = null
  fileContent.value = ''
  originalContent.value = ''
  isEditing.value = false
  loadFiles()
})

onMounted(() => {
  loadAgents()
})
</script>

<template>
  <div class="flex h-full">
    <!-- Left: Agent list -->
    <div class="w-56 shrink-0 border-r border-border flex flex-col">
      <div class="p-3 text-sm font-medium text-muted-foreground border-b border-border">
        {{ t('memory.title') }}
      </div>
      <div class="flex-1 overflow-y-auto">
        <button
          v-for="agent in agents"
          :key="agent.id"
          :class="cn(
            'flex w-full items-center gap-2 px-3 py-2 text-sm transition-colors hover:bg-muted/50',
            selectedAgentId === agent.id && 'bg-muted'
          )"
          @click="selectedAgentId = agent.id"
        >
          <img
            v-if="agent.icon"
            :src="agent.icon"
            class="size-6 shrink-0 rounded"
            alt=""
          />
          <div v-else class="size-6 shrink-0 rounded bg-muted flex items-center justify-center text-xs text-muted-foreground">
            {{ agent.name.charAt(0) }}
          </div>
          <span class="truncate">{{ agent.name }}</span>
        </button>
      </div>
    </div>

    <!-- Right: Memory content -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- No agent selected -->
      <div v-if="!selectedAgent" class="flex-1 flex items-center justify-center text-muted-foreground text-sm">
        {{ t('memory.selectAgent') }}
      </div>

      <!-- Agent has no OpenClaw workspace -->
      <div v-else-if="!hasWorkspace" class="flex-1 flex items-center justify-center text-muted-foreground text-sm">
        <div class="text-center space-y-2">
          <FolderOpen class="size-10 mx-auto opacity-40" />
          <p>{{ t('memory.noWorkspace') }}</p>
        </div>
      </div>

      <!-- Workspace content -->
      <template v-else>
        <div class="flex h-full">
          <!-- File list -->
          <div class="w-52 shrink-0 border-r border-border flex flex-col">
            <div class="flex items-center justify-between p-2 border-b border-border">
              <span class="text-xs font-medium text-muted-foreground">{{ t('memory.files') }}</span>
              <button
                class="p-1 rounded hover:bg-muted text-muted-foreground"
                :title="t('memory.newFile')"
                @click="showNewFileDialog = true"
              >
                <Plus class="size-3.5" />
              </button>
            </div>
            <div class="flex-1 overflow-y-auto">
              <template v-for="cat in ['core', 'persona', 'daily', 'other']" :key="cat">
                <template v-if="memoryFiles.filter(f => f.category === cat).length">
                  <div class="px-2 pt-2 pb-1 text-[11px] font-medium text-muted-foreground/70 uppercase tracking-wider">
                    {{ categoryLabel(cat) }}
                  </div>
                  <button
                    v-for="file in memoryFiles.filter(f => f.category === cat)"
                    :key="file.path"
                    :class="cn(
                      'flex w-full items-center gap-1.5 px-2 py-1.5 text-xs transition-colors hover:bg-muted/50',
                      selectedFilePath === file.path && 'bg-muted'
                    )"
                    @click="selectFile(file.path)"
                  >
                    <span>{{ categoryIcon(file.category) }}</span>
                    <span class="truncate">{{ file.name }}</span>
                  </button>
                </template>
              </template>
              <div v-if="!memoryFiles.length && !isLoading" class="p-3 text-xs text-muted-foreground text-center">
                {{ t('memory.noFiles') }}
              </div>
            </div>
          </div>

          <!-- Editor -->
          <div class="flex-1 flex flex-col min-w-0">
            <div v-if="!selectedFile" class="flex-1 flex items-center justify-center text-muted-foreground text-sm">
              <div class="text-center space-y-2">
                <FileText class="size-8 mx-auto opacity-40" />
                <p>{{ t('memory.selectFile') }}</p>
              </div>
            </div>
            <template v-else>
              <!-- Toolbar -->
              <div class="flex items-center justify-between px-3 py-2 border-b border-border">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="text-sm font-medium truncate">{{ selectedFile.path }}</span>
                  <span v-if="hasChanges" class="text-[10px] text-amber-500 shrink-0">●</span>
                </div>
                <div class="flex items-center gap-1 shrink-0">
                  <template v-if="isEditing">
                    <button
                      class="px-2 py-1 text-xs rounded bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                      :disabled="!hasChanges"
                      @click="saveFile"
                    >
                      <Save class="size-3.5 inline mr-1" />{{ t('memory.save') }}
                    </button>
                    <button
                      class="px-2 py-1 text-xs rounded hover:bg-muted text-muted-foreground"
                      @click="cancelEdit"
                    >
                      {{ t('memory.cancel') }}
                    </button>
                  </template>
                  <template v-else>
                    <button
                      class="px-2 py-1 text-xs rounded hover:bg-muted text-muted-foreground"
                      @click="isEditing = true"
                    >
                      {{ t('memory.edit') }}
                    </button>
                    <button
                      class="px-2 py-1 text-xs rounded hover:bg-muted text-destructive"
                      @click="showDeleteDialog = true"
                    >
                      <Trash2 class="size-3.5" />
                    </button>
                  </template>
                </div>
              </div>
              <!-- Content area -->
              <div class="flex-1 overflow-y-auto">
                <textarea
                  v-if="isEditing"
                  v-model="fileContent"
                  class="w-full h-full p-4 text-sm font-mono bg-transparent resize-none outline-none"
                  spellcheck="false"
                />
                <pre
                  v-else
                  class="p-4 text-sm font-mono whitespace-pre-wrap break-words"
                >{{ fileContent }}</pre>
              </div>
            </template>
          </div>
        </div>
      </template>
    </div>

    <!-- New file dialog -->
    <AlertDialog :open="showNewFileDialog" @update:open="showNewFileDialog = $event">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('memory.newFile') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('memory.newFileDesc') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <input
          v-model="newFileName"
          class="w-full px-3 py-2 text-sm rounded-md border border-border bg-transparent outline-none focus:ring-1 focus:ring-ring"
          :placeholder="t('memory.newFilePlaceholder')"
          @keydown.enter="createNewFile"
        />
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('memory.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            :disabled="!newFileName.trim()"
            @click.prevent="createNewFile"
          >
            {{ t('memory.create') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- Delete confirm dialog -->
    <AlertDialog :open="showDeleteDialog" @update:open="showDeleteDialog = $event">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('memory.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('memory.deleteFileConfirm', { name: selectedFile?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('memory.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click.prevent="deleteFile"
          >
            {{ t('memory.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
