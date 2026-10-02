<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted, ref, watch } from 'vue'
import { useI18n } from '../../i18n'
import { usePreferencesStore } from '../../stores/preferences'

const { t } = useI18n()
const preferencesStore = usePreferencesStore()
const { preferences, loading, saving, error, message } = storeToRefs(preferencesStore)
const interval = ref(100)

onMounted(async () => {
  await preferencesStore.load()
  interval.value = preferences.value.batchIntervalMs
})

watch(
  () => preferences.value.batchIntervalMs,
  (value) => {
    interval.value = value
  },
)

async function save() {
  await preferencesStore.saveBatchInterval(interval.value)
}
</script>

<template>
  <section class="panel" aria-labelledby="settings-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ t('settings.eyebrow') }}</p>
        <h2 id="settings-title">{{ t('settings.title') }}</h2>
      </div>
    </div>

    <form class="settings-form" @submit.prevent="save">
      <label for="batch-interval">{{ t('settings.batch_interval') }}</label>
      <div class="input-row">
        <input
          id="batch-interval"
          v-model.number="interval"
          type="number"
          min="50"
          max="1000"
          step="50"
          :disabled="loading || saving"
        >
        <span>{{ t('settings.milliseconds') }}</span>
        <button type="submit" :disabled="loading || saving">{{ t('common.save') }}</button>
      </div>
    </form>

    <p class="hint">{{ t('settings.hint') }}</p>
    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>
    <p v-if="message" class="success" aria-live="polite">{{ message }}</p>
  </section>
</template>
