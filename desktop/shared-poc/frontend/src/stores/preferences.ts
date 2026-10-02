import { defineStore } from 'pinia'
import { ref } from 'vue'
import { clientError, desktopApi, type APIError, type Preferences } from '../api/desktopApi'

const fallback: Preferences = {
  batchIntervalMs: 100,
  windowWidth: 1080,
  windowHeight: 720,
}

export const usePreferencesStore = defineStore('preferences', () => {
  const preferences = ref<Preferences>({ ...fallback })
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<APIError | null>(null)
  const message = ref('')

  async function load() {
    loading.value = true
    error.value = null
    try {
      const result = await desktopApi.getPreferences()
      preferences.value = { ...result.preferences }
      error.value = result.error ?? null
    } catch (caught) {
      error.value = clientError('preferences_call_failed', caught)
    } finally {
      loading.value = false
    }
  }

  async function saveBatchInterval(batchIntervalMs: number) {
    saving.value = true
    error.value = null
    message.value = ''
    const next = { ...preferences.value, batchIntervalMs }
    try {
      const result = await desktopApi.savePreferences(next)
      error.value = result.error ?? null
      if (!error.value && result.saved) {
        preferences.value = next
        message.value = 'POC preference saved.'
      }
      return result.saved && !result.error
    } catch (caught) {
      error.value = clientError('preferences_save_call_failed', caught)
      return false
    } finally {
      saving.value = false
    }
  }

  return { preferences, loading, saving, error, message, load, saveBatchInterval }
})
