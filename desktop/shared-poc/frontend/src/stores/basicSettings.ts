import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import { clientError, desktopApi, type APIError, type BasicSettings } from '../api/desktopApi'
import { settingsPayload, validBasicSettings } from '../features/settingsLogic'
export const useBasicSettingsStore = defineStore('basicSettings', () => {
  const current = shallowRef<BasicSettings | null>(null)
  const draft = ref<BasicSettings>({ port: 0, logLevel: 'info', coreAutostart: false })
  const mutable = shallowRef(false)
  const busy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  const completed = shallowRef(false)
  const valid = computed(() => current.value !== null && validBasicSettings(draft.value))
  function reset() {
    if (current.value && !busy.value) draft.value = { ...current.value }
    completed.value = false
  }
  async function readSettings() {
    const result = await desktopApi.readBasicSettings()
    error.value = result.error ?? null
    if (error.value) { current.value = null; mutable.value = false; return }
    current.value = result.settings; mutable.value = result.coreAutostartMutable
    draft.value = { ...result.settings }
  }
  async function load() {
    if (busy.value) return
    busy.value = true; completed.value = false; error.value = null
    try { await readSettings() }
    catch (caught) { current.value = null; mutable.value = false; error.value = clientError('settings_read_call_failed', caught) }
    finally { busy.value = false }
  }
  function payload() {
    return current.value ? settingsPayload(draft.value, current.value, mutable.value) : null
  }
  async function save(confirmed: BasicSettings) {
    if (busy.value || !current.value || !validBasicSettings(confirmed)) return
    busy.value = true; completed.value = false; error.value = null
    try {
      const result = await desktopApi.saveBasicSettings(settingsPayload(confirmed, current.value, mutable.value))
      error.value = result.error ?? (!result.completed ? clientError('settings_not_completed', '') : null)
      if (!error.value) { completed.value = true; await readSettings() }
    } catch (caught) { error.value = clientError('settings_save_call_failed', caught) }
    finally { busy.value = false }
  }
  return { current, draft, mutable, busy, error, completed, valid, reset, load, payload, save }
})
