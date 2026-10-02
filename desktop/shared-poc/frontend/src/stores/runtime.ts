import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { desktopApi, type APIError, type ServiceStatus } from '../api/desktopApi'

const emptyStatus = (): ServiceStatus => ({
  running: false,
  healthy: false,
  startupEnabled: false,
  nexusConnected: false,
})

export const useRuntimeStore = defineStore('runtime', () => {
  const status = ref<ServiceStatus>(emptyStatus())
  const runtimeRoot = ref('')
  const loading = ref(false)
  const actionPending = ref<'start' | 'stop' | 'restart' | null>(null)
  const error = ref<APIError | null>(null)
  const announcement = ref('Runtime status not loaded yet.')

  const stateLabel = computed(() => {
    if (loading.value) return 'Loading'
    if (error.value) return 'Unavailable'
    if (!status.value.running) return 'Stopped'
    return status.value.healthy ? 'Healthy' : 'Running'
  })

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const result = await desktopApi.runtimeStatus()
      runtimeRoot.value = result.runtimeRoot || ''
      status.value = result.status ?? emptyStatus()
      error.value = result.error ?? null
      announcement.value = error.value
        ? `Runtime status failed: ${error.value.message}`
        : `Runtime status: ${stateLabel.value}.`
    } catch (caught) {
      error.value = {
        code: 'runtime_call_failed',
        message: caught instanceof Error ? caught.message : String(caught),
      }
      announcement.value = `Runtime status failed: ${error.value.message}`
    } finally {
      loading.value = false
    }
  }

  async function perform(action: 'start' | 'stop' | 'restart') {
    actionPending.value = action
    error.value = null
    try {
      const result = await desktopApi.runtimeAction(action)
      error.value = result.error ?? null
      announcement.value = error.value
        ? `${action} failed: ${error.value.message}`
        : `${action} completed.`
      if (!error.value) await refresh()
    } catch (caught) {
      error.value = {
        code: 'runtime_action_call_failed',
        message: caught instanceof Error ? caught.message : String(caught),
      }
      announcement.value = `${action} failed: ${error.value.message}`
    } finally {
      actionPending.value = null
    }
  }

  return {
    status,
    runtimeRoot,
    loading,
    actionPending,
    error,
    announcement,
    stateLabel,
    refresh,
    perform,
  }
})
