<script setup lang="ts">
/**
 * Update dialog with two modes:
 * - 'new-version': shown when a new version is detected, with "Install & Restart" button
 * - 'just-updated': shown on first launch after update, with "Got it" button
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { UpdaterService } from '@bindings/willclaw/internal/services/updater'
import { toast } from '@/components/ui/toast'
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'

const props = defineProps<{
  open: boolean
  mode: 'new-version' | 'just-updated'
  version: string
  releaseNotes: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()

const isInstalling = ref(false)

// Format version for display (ensure "v" prefix)
function displayVersion(v: string): string {
  return v.startsWith('v') || v.startsWith('V') ? v : `V${v}`
}

// Install and restart
async function handleInstallAndRestart() {
  if (isInstalling.value) return
  isInstalling.value = true
  try {
    await UpdaterService.DownloadAndApply()
    await UpdaterService.RestartApp()
  } catch (error) {
    console.error('Failed to install update:', error)
    toast.error(t('settings.about.updateFailed'))
    isInstalling.value = false
  }
}

function handleClose() {
  if (isInstalling.value) return
  emit('update:open', false)
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="lg" :show-close-button="!isInstalling">
      <DialogHeader>
        <DialogTitle>
          <template v-if="mode === 'new-version'">
            {{ t('settings.about.updateDialogTitle', { version: displayVersion(version) }) }}
          </template>
          <template v-else>
            {{ t('settings.about.updatedTitle', { version: displayVersion(version) }) }}
          </template>
        </DialogTitle>
        <p class="mt-1 text-sm text-muted-foreground">
          <template v-if="mode === 'new-version'">
            {{ t('settings.about.updateDialogSubtitle') }}
          </template>
          <template v-else>
            {{ t('settings.about.updatedSubtitle') }}
          </template>
        </p>
      </DialogHeader>

      <!-- Release notes -->
      <div
        v-if="releaseNotes"
        class="my-4 max-h-[400px] overflow-y-auto rounded-lg border border-border bg-muted/30 p-4"
      >
        <MarkdownRenderer :content="releaseNotes" />
      </div>

      <DialogFooter>
        <template v-if="mode === 'new-version'">
          <Button
            :disabled="isInstalling"
            @click="handleInstallAndRestart"
          >
            <LoaderCircle v-if="isInstalling" class="mr-2 size-4 animate-spin" />
            {{ isInstalling ? t('settings.about.installing') : t('settings.about.installAndRestart') }}
          </Button>
        </template>
        <template v-else>
          <Button @click="handleClose">
            {{ t('settings.about.gotIt') }}
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
