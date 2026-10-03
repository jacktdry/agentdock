import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'
import { clientError, desktopApi, type APIError, type ConnectionActionName, type ConnectionStatus } from '../api/desktopApi'
import { canPerformConnection, safePublicURL } from '../features/connectionLogic'
export const useConnectionStore = defineStore('connection', () => {
  const status = shallowRef<ConnectionStatus | null>(null)
  const busy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  const completed = shallowRef(false)
  const publicURL = computed(() => safePublicURL(status.value?.publicURL))
  function canPerform(action: ConnectionActionName) { return !busy.value && canPerformConnection(action, status.value) }
  async function readStatus() {
    const result = await desktopApi.connectionStatus()
    error.value = result.error ?? null
    status.value = error.value ? null : result.status
  }
  async function refresh() {
    if (busy.value) return
    busy.value = true; completed.value = false; error.value = null
    try { await readStatus() }
    catch (caught) { status.value = null; error.value = clientError('connection_call_failed', caught) }
    finally { busy.value = false }
  }
  async function perform(action: ConnectionActionName) {
    if (!canPerform(action)) return
    busy.value = true; completed.value = false; error.value = null
    try {
      const result = await desktopApi.connectionAction(action)
      error.value = result.error ?? (!result.completed ? clientError('connection_not_completed', '') : null)
      if (!error.value) { completed.value = true; await readStatus() }
    } catch (caught) { error.value = clientError('connection_action_call_failed', caught) }
    finally { busy.value = false }
  }
  return { status, busy, error, completed, publicURL, canPerform, refresh, perform }
})
