<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { GreetService } from '@bindings/willchat/internal/services/greet'
import { WindowService } from '@bindings/willchat/internal/services/windows'

defineProps<{ msg: string }>()

const { t } = useI18n()

const name = ref('')
const result = ref('')
const time = ref('')
const settingsVisible = ref(false)

// 初始化显示文本
result.value = t('hello.inputPlaceholder')
time.value = t('hello.listeningEvent')

const doGreet = () => {
  let localName = name.value
  if (!localName) {
    localName = t('hello.defaultName')
  }
  GreetService.Greet(localName)
    .then((resultValue: string) => {
      result.value = resultValue
    })
    .catch((err: Error) => {
      console.log(err)
    })
}

onMounted(() => {
  Events.On('time', (timeValue: { data: string }) => {
    time.value = timeValue.data
  })

  WindowService.IsVisible('settings')
    .then((v: boolean) => {
      settingsVisible.value = v
    })
    .catch(() => {
      settingsVisible.value = false
    })
})

const showSettings = () => {
  WindowService.Show('settings')
    .then(() => {
      settingsVisible.value = true
    })
    .catch((err: Error) => console.log(err))
}

const hideSettings = () => {
  WindowService.Close('settings')
    .then(() => {
      settingsVisible.value = false
    })
    .catch((err: Error) => console.log(err))
}
</script>

<template>
  <h1>{{ msg }}</h1>

  <div aria-label="result" class="result">{{ result }}</div>
  <div class="card">
    <div class="input-box">
      <input aria-label="input" class="input" v-model="name" type="text" autocomplete="off" />
      <button aria-label="greet-btn" class="btn" @click="doGreet">
        {{ t('hello.greetButton') }}
      </button>
    </div>
  </div>

  <div class="card" style="margin-top: 12px">
    <div style="display: flex; gap: 8px; flex-wrap: wrap">
      <button class="btn" @click="showSettings">{{ t('hello.showSettings') }}</button>
      <button class="btn" @click="hideSettings" :disabled="!settingsVisible">
        {{ t('hello.hideSettings') }}
      </button>
    </div>
  </div>

  <div class="footer">
    <div>
      <p>{{ t('hello.learnMore') }}</p>
    </div>
    <div>
      <p>{{ time }}</p>
    </div>
  </div>
</template>
