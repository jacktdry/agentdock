import { defineStore } from 'pinia'
import { shallowRef } from 'vue'
import { clientError, desktopApi, type APIError, type DiagnosticsSnapshot } from '../api/desktopApi'
export const useDiagnosticsStore = defineStore('diagnostics', () => {
  const data = shallowRef<DiagnosticsSnapshot | null>(null)
  const busy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  async function refresh() {
    if (busy.value) return
    busy.value = true; error.value = null
    try {
      const result = await desktopApi.diagnostics()
      error.value = result.error ?? null
      data.value = error.value ? null : result.snapshot
    } catch (caught) { data.value = null; error.value = clientError('diagnostics_call_failed', caught) }
    finally { busy.value = false }
  }
  return { data, busy, error, refresh }
})
