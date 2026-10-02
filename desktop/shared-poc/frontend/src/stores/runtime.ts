import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  clientError,
  desktopApi,
  type APIError,
  type RuntimeActionName,
  type RuntimeStatus,
} from '../api/desktopApi'
import { t, type MessageKey } from '../i18n'

const emptyStatus = (): RuntimeStatus => ({
  running: false,
  healthy: false,
  startupEnabled: false,
  nexusConnected: false,
})

const actionMessageKeys: Record<RuntimeActionName, MessageKey> = {
  start: 'common.start',
  restart: 'common.restart',
  stop: 'common.stop',
}

export const useRuntimeStore = defineStore('runtime', () => {
  const status = ref<RuntimeStatus>(emptyStatus())
  const runtimeRoot = ref('')
  const loading = ref(false)
  const actionPending = ref<RuntimeActionName | null>(null)
  const error = ref<APIError | null>(null)
  const announcement = ref(t('runtime.status_not_loaded'))

  const stateKind = computed(() => {
    if (loading.value) return 'loading'
    if (error.value) return 'unavailable'
    if (!status.value.running) return 'stopped'
    return status.value.healthy ? 'healthy' : 'running'
  })

  const stateLabel = computed(() => {
    switch (stateKind.value) {
      case 'loading':
        return t('common.loading')
      case 'unavailable':
        return t('common.unavailable')
      case 'stopped':
        return t('common.stopped')
      case 'healthy':
        return t('common.healthy')
      default:
        return t('common.running')
    }
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
        ? t('runtime.status_failed', { message: error.value.message })
        : t('runtime.status_summary', { state: stateLabel.value })
    } catch (caught) {
      error.value = clientError('runtime_call_failed', caught)
      announcement.value = t('runtime.status_failed', { message: error.value.message })
    } finally {
      loading.value = false
    }
  }

  async function perform(action: RuntimeActionName) {
    actionPending.value = action
    error.value = null
    const actionLabel = t(actionMessageKeys[action])
    try {
      const result = await desktopApi.runtimeAction(action)
      error.value = result.error ?? null
      announcement.value = error.value
        ? t('runtime.action_failed', { action: actionLabel, message: error.value.message })
        : t('runtime.action_completed', { action: actionLabel })
      if (!error.value) await refresh()
    } catch (caught) {
      error.value = clientError('runtime_action_call_failed', caught)
      announcement.value = t('runtime.action_failed', {
        action: actionLabel,
        message: error.value.message,
      })
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
    stateKind,
    stateLabel,
    refresh,
    perform,
  }
})
