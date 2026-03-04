<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { FolderPlus } from 'lucide-vue-next'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import type { Folder } from '@bindings/chatclaw/internal/services/library'

const props = defineProps<{
  libraryId: number
  folders: Folder[]
  modelValue: number | null // null = 全部, -1 = 未分组, >0 = 文件夹ID
}>()

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
  'create-folder': []
  'rename-folder': [folder: Folder]
  'delete-folder': [folder: Folder]
}>()

const { t } = useI18n()

const selectedValue = computed({
  get: () => {
    if (props.modelValue === null) return 'all'
    if (props.modelValue === -1) return 'uncategorized'
    return String(props.modelValue)
  },
  set: (val: string) => {
    if (val === 'all') {
      emit('update:modelValue', null)
    } else if (val === 'uncategorized') {
      emit('update:modelValue', -1)
    } else {
      emit('update:modelValue', Number(val))
    }
  },
})

// 递归展平文件夹树，用于显示
const flattenFolders = (folders: Folder[], prefix = ''): Array<Folder & { displayName: string }> => {
  const result: Array<Folder & { displayName: string }> = []
  for (const folder of folders) {
    const displayName = prefix ? `${prefix} / ${folder.name}` : folder.name
    result.push({ ...folder, displayName })
    if (folder.children && folder.children.length > 0) {
      result.push(...flattenFolders(folder.children, displayName))
    }
  }
  return result
}

const flatFolders = computed(() => flattenFolders(props.folders))

const displayText = computed(() => {
  if (props.modelValue === null) return t('knowledge.folder.all')
  if (props.modelValue === -1) return t('knowledge.folder.uncategorized')
  const folder = flatFolders.value.find((f) => f.id === props.modelValue)
  return folder?.displayName || ''
})
</script>

<template>
  <div class="flex items-center gap-2">
    <Select v-model="selectedValue">
      <SelectTrigger class="h-7 w-[140px] text-xs">
        <SelectValue :placeholder="displayText" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{{ t('knowledge.folder.all') }}</SelectItem>
        <SelectItem value="uncategorized">{{ t('knowledge.folder.uncategorized') }}</SelectItem>
        <template v-for="folder in flatFolders" :key="folder.id">
          <div class="group relative">
            <SelectItem :value="String(folder.id)">{{ folder.displayName }}</SelectItem>
            <DropdownMenu>
              <DropdownMenuTrigger
                class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100"
                @click.stop
              >
                <div class="size-4" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" class="w-auto min-w-max">
                <DropdownMenuItem class="whitespace-nowrap" @select="emit('rename-folder', folder)">
                  {{ t('knowledge.folder.rename') }}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  class="whitespace-nowrap text-muted-foreground focus:text-foreground"
                  @select="emit('delete-folder', folder)"
                >
                  {{ t('knowledge.folder.delete') }}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </template>
      </SelectContent>
    </Select>
    <Button
      variant="ghost"
      size="icon"
      class="h-7 w-7"
      :title="t('knowledge.folder.create')"
      @click="emit('create-folder')"
    >
      <FolderPlus class="size-4 text-muted-foreground" />
    </Button>
  </div>
</template>
