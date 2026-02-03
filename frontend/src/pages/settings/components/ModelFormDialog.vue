<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Model } from '@/../bindings/willchat/internal/services/providers'

const props = defineProps<{
  open: boolean
  model?: Model | null // 编辑时传入，添加时不传
  providerName: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  save: [data: { modelId: string; name: string; type: string }]
}>()

const { t } = useI18n()

// 表单数据
const formModelId = ref('')
const formName = ref('')
const formType = ref<string>('llm')
const isSaving = ref(false)

// 是否为编辑模式
const isEdit = computed(() => !!props.model)

// 对话框标题
const dialogTitle = computed(() =>
  isEdit.value ? t('settings.modelService.editModel') : t('settings.modelService.addModel')
)

// 表单验证
const isFormValid = computed(() => {
  return formModelId.value.trim() !== '' && formName.value.trim() !== '' && formType.value !== ''
})

// 监听 model 变化，初始化表单
watch(
  () => props.model,
  (model) => {
    if (model) {
      formModelId.value = model.model_id
      formName.value = model.name
      formType.value = model.type
    } else {
      formModelId.value = ''
      formName.value = ''
      formType.value = 'llm'
    }
  },
  { immediate: true }
)

// 监听 open 变化，重置表单
watch(
  () => props.open,
  (open) => {
    if (open && !props.model) {
      formModelId.value = ''
      formName.value = ''
      formType.value = 'llm'
    }
  }
)

// 处理关闭
const handleClose = () => {
  emit('update:open', false)
}

// 处理保存
const handleSave = () => {
  if (!isFormValid.value) return
  isSaving.value = true
  emit('save', {
    modelId: formModelId.value.trim(),
    name: formName.value.trim(),
    type: formType.value,
  })
}

// 暴露方法让父组件重置保存状态
const resetSaving = () => {
  isSaving.value = false
}

defineExpose({ resetSaving })
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="sm">
      <DialogHeader>
        <DialogTitle>{{ dialogTitle }}</DialogTitle>
        <DialogDescription>
          {{ providerName }}
        </DialogDescription>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <!-- 模型类型 -->
        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('settings.modelService.modelType') }}
            <span v-if="!isEdit" class="text-destructive">*</span>
          </label>
          <Select v-model="formType" :disabled="isSaving || isEdit">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="llm">
                {{ t('settings.modelService.llmModels') }}
              </SelectItem>
              <SelectItem value="embedding">
                {{ t('settings.modelService.embeddingModels') }}
              </SelectItem>
              <SelectItem value="rerank">
                {{ t('settings.modelService.rerankModels') }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <!-- 模型 ID -->
        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('settings.modelService.modelId') }}
            <span v-if="!isEdit" class="text-destructive">*</span>
          </label>
          <Input
            v-model="formModelId"
            :placeholder="t('settings.modelService.modelIdPlaceholder')"
            :disabled="isSaving || isEdit"
            maxlength="40"
          />
        </div>

        <!-- 模型名称 -->
        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('settings.modelService.modelName') }}
            <span class="text-destructive">*</span>
          </label>
          <Input
            v-model="formName"
            :placeholder="t('settings.modelService.modelNamePlaceholder')"
            :disabled="isSaving"
            maxlength="40"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="isSaving" @click="handleClose">
          {{ t('settings.modelService.cancel') }}
        </Button>
        <Button :disabled="!isFormValid || isSaving" @click="handleSave">
          <LoaderCircle v-if="isSaving" class="mr-2 size-4 animate-spin" />
          {{ t('settings.modelService.save') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
