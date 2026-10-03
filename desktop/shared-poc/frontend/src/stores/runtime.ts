import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  clientError,
  desktopApi,
  type APIError,
  type RuntimeActionName,
  type RuntimeStatus,
} from '../api/desktopApi'
import { errorMessage } from '../api/errorMessage'
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
    if (loading.value || actionPending.value) return
    loading.value = true
    error.value = null
    try {
      const result = await desktopApi.runtimeStatus()
      runtimeRoot.value = result.runtimeRoot || ''
      status.value = result.status ?? emptyStatus()
      error.value = result.error ?? null
      announcement.value = error.value
        ? t('runtime.status_failed', { message: errorMessage(error.value) })
        : t('runtime.status_summary', { state: stateLabel.value })
    } catch (caught) {
      error.value = clientError('runtime_call_failed', caught)
      announcement.value = t('runtime.status_failed', { message: errorMessage(error.value) })
    } finally {
      loading.value = false
    }
  }

  async function perform(action: RuntimeActionName) {
    if (loading.value || actionPending.value) return
    actionPending.value = action
    error.value = null
    const actionLabel = t(actionMessageKeys[action])
    try {
      const result = await desktopApi.runtimeAction(action)
      error.value = result.error ?? null
      announcement.value = error.value
        ? t('runtime.action_failed', { action: actionLabel, message: errorMessage(error.value) })
        : t('runtime.action_completed', { action: actionLabel })
      if (!error.value) {
        actionPending.value = null
        await refresh()
      }
    } catch (caught) {
      error.value = clientError('runtime_action_call_failed', caught)
      announcement.value = t('runtime.action_failed', {
        action: actionLabel,
        message: errorMessage(error.value),
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
