<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { PopoverRoot, PopoverTrigger, PopoverContent, PopoverPortal } from 'reka-ui'
import { Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  modelValue: string
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()
const open = ref(false)
const search = ref('')

const EMOJI_CATEGORIES: Record<string, string[]> = {
  faces: [
    '😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂', '🙂', '😉',
    '😊', '😇', '🥰', '😍', '🤩', '😘', '😋', '😛', '🤔', '🤫',
    '🤨', '😐', '😑', '😶', '🙄', '😏', '😣', '😥', '😮', '🤐',
    '😯', '😪', '😫', '🥱', '😴', '🤤', '😌', '😷', '🤒', '🤕',
    '🤢', '🤮', '🤧', '🥵', '🥶', '😵', '🤯', '🥳', '🤠', '🥸',
    '😎', '🤓', '🧐', '😈', '👿', '👹', '👺', '💀', '👻', '👽',
    '🤖', '💩', '🎃',
  ],
  animals: [
    '🐶', '🐱', '🐭', '🐹', '🐰', '🦊', '🐻', '🐼', '🐨', '🐯',
    '🦁', '🐮', '🐷', '🐸', '🐵', '🙈', '🙉', '🙊', '🐔', '🐧',
    '🐦', '🦅', '🦆', '🦉', '🐝', '🐛', '🦋', '🐌', '🐞', '🐜',
    '🦥', '🦦', '🦨', '🦘', '🦡', '🐾', '🦖', '🐉', '🦕', '🐙',
    '🐠', '🐬', '🐳', '🦈', '🐚', '🦀', '🦑', '🐡',
  ],
  nature: [
    '🌸', '💐', '🌷', '🌹', '🥀', '🌺', '🌻', '🌼', '🌿', '🍀',
    '🍁', '🍂', '🍃', '🌾', '🌵', '🌲', '🌳', '🌴', '☘️', '🍄',
    '🌍', '🌎', '🌏', '🌙', '⭐', '🌟', '✨', '⚡', '🔥', '🌈',
    '☀️', '🌤️', '⛅', '🌧️', '❄️', '💧', '🌊',
  ],
  food: [
    '🍎', '🍊', '🍋', '🍌', '🍉', '🍇', '🍓', '🫐', '🍈', '🍒',
    '🍑', '🥝', '🍍', '🥥', '🥑', '🍆', '🥕', '🌽', '🌶️', '🥒',
    '🥦', '🍔', '🍕', '🌭', '🍟', '🍿', '🧁', '🍰', '🎂', '🍩',
    '🍪', '🍫', '🍬', '☕', '🍵', '🧃', '🥤', '🧋',
  ],
  objects: [
    '💻', '🖥️', '⌨️', '🖱️', '💾', '💿', '📱', '📲', '☎️', '📞',
    '📟', '📠', '🔋', '🔌', '💡', '🔦', '📷', '📹', '🎥', '📽️',
    '🎬', '📺', '📻', '🎙️', '🎚️', '🎛️', '⏱️', '⏰', '🔬', '🔭',
    '📡', '🧲', '🔧', '🔨', '⚙️', '🛠️', '⚔️', '🔫', '🛡️', '🗝️',
    '🔑', '📦', '📮', '✉️', '📝', '📚', '📖',
  ],
  symbols: [
    '❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍', '🤎', '💔',
    '💕', '💗', '💖', '💘', '💝', '💟', '🏳️', '🏴', '🚩', '🎌',
    '⚠️', '🚫', '⛔', '❌', '⭕', '✅', '❓', '❗', '💯', '🔴',
    '🟠', '🟡', '🟢', '🔵', '🟣', '⚫', '⚪', '🟤', '♻️', '🎵',
    '🎶', '🔔', '🏷️', '🔖',
  ],
  activities: [
    '⚽', '🏀', '🏈', '⚾', '🥎', '🎾', '🏐', '🏉', '🥏', '🎱',
    '🏓', '🏸', '🥊', '🥋', '⛳', '🎯', '🎳', '🏆', '🥇', '🥈',
    '🥉', '🏅', '🎖️', '🎪', '🎭', '🎨', '🎬', '🎤', '🎧', '🎼',
    '🎹', '🥁', '🎷', '🎺', '🎸', '🪕', '🎻', '🎲', '🧩', '🎮',
    '🎰', '🧸',
  ],
}

const CATEGORY_ICONS: Record<string, string> = {
  faces: '😀',
  animals: '🐱',
  nature: '🌸',
  food: '🍎',
  objects: '💻',
  symbols: '❤️',
  activities: '⚽',
}

const activeCategory = ref('faces')

const filteredEmojis = computed(() => {
  if (!search.value.trim()) {
    return EMOJI_CATEGORIES[activeCategory.value] ?? []
  }
  const all = Object.values(EMOJI_CATEGORIES).flat()
  return all
})

const selectEmoji = (emoji: string) => {
  emit('update:modelValue', emoji)
  open.value = false
  search.value = ''
}

const clearEmoji = () => {
  emit('update:modelValue', '')
}
</script>

<template>
  <div class="flex items-center gap-2">
    <PopoverRoot v-model:open="open">
      <PopoverTrigger as-child>
        <button
          type="button"
          class="flex h-9 w-full items-center gap-2 rounded-md border border-border bg-background px-3 text-sm transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          <span v-if="modelValue" class="text-xl leading-none">{{ modelValue }}</span>
          <span :class="modelValue ? 'text-foreground' : 'text-muted-foreground'">
            {{ modelValue ? t('assistant.fields.identityEmojiChange') : (placeholder || t('assistant.fields.identityEmojiPlaceholder')) }}
          </span>
        </button>
      </PopoverTrigger>
      <PopoverPortal>
        <PopoverContent
          side="bottom"
          align="start"
          :side-offset="4"
          class="z-50 w-[320px] rounded-lg border border-border bg-popover p-0 shadow-md dark:shadow-none dark:ring-1 dark:ring-white/10 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
        >
          <div class="flex items-center gap-1 border-b border-border px-2 py-1.5">
            <button
              v-for="(icon, cat) in CATEGORY_ICONS"
              :key="cat"
              type="button"
              class="rounded p-1.5 text-base transition-colors"
              :class="activeCategory === cat
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'"
              @click="activeCategory = cat as string; search = ''"
            >
              {{ icon }}
            </button>
          </div>

          <div class="grid max-h-[200px] grid-cols-8 gap-0.5 overflow-y-auto p-2">
            <button
              v-for="emoji in filteredEmojis"
              :key="emoji"
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded text-lg transition-colors hover:bg-accent"
              @click="selectEmoji(emoji)"
            >
              {{ emoji }}
            </button>
          </div>
        </PopoverContent>
      </PopoverPortal>
    </PopoverRoot>

    <Button
      v-if="modelValue"
      size="icon"
      variant="ghost"
      class="h-8 w-8 shrink-0"
      @click="clearEmoji"
    >
      <Trash2 class="size-4" />
    </Button>
  </div>
</template>
