<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted, ref, watch } from 'vue'
import { usePreferencesStore } from '../../stores/preferences'

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
        <p class="eyebrow">Persistence probe</p>
        <h2 id="settings-title">POC settings</h2>
      </div>
    </div>

    <form class="settings-form" @submit.prevent="save">
      <label for="batch-interval">Event batch interval</label>
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
        <span>ms</span>
        <button type="submit" :disabled="loading || saving">Save</button>
      </div>
    </form>

    <p class="hint">Stored outside the production AgentDock configuration.</p>
    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>
    <p v-if="message" class="success" aria-live="polite">{{ message }}</p>
  </section>
</template>
