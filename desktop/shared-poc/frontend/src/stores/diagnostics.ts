import { defineStore } from 'pinia'
import { shallowRef } from 'vue'
import { clientError, desktopApi, type APIError, type DiagnosticsSnapshot, type DiagnosticsDirectories } from '../api/desktopApi'
export const useDiagnosticsStore = defineStore('diagnostics', () => {
  const data = shallowRef<DiagnosticsSnapshot | null>(null)
  const directories = shallowRef<DiagnosticsDirectories | null>(null)
  const opening = shallowRef(false)
  const openError = shallowRef<APIError | null>(null)
  const opened = shallowRef(false)
  const busy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  async function refresh() {
    if (busy.value || opening.value) return
    busy.value = true; error.value = null
    directories.value = null; opened.value = false; openError.value = null
    try {
      const result = await desktopApi.diagnostics()
      const capabilities = await desktopApi.diagnosticsDirectories()
      const safeCapability = (value: DiagnosticsDirectories['logs']) => ({
        enabled: value.enabled === true,
        reason: value.reason === 'requires_native' ? 'requires_native' : value.enabled === true ? '' : 'unavailable',
      })
      directories.value = { logs: safeCapability(capabilities.logs), configuration: safeCapability(capabilities.configuration) }
      error.value = result.error ?? null
      data.value = error.value ? null : result.snapshot
    } catch { data.value = null; error.value = clientError('diagnostics_call_failed', new Error()); directories.value = null }
    finally { busy.value = false }
  }
  async function openDirectory(kind: 'logs' | 'configuration') {
    if (busy.value || opening.value || !directories.value?.[kind].enabled) return
    opening.value = true; openError.value = null; opened.value = false
    try {
      const result = await desktopApi.openNextDirectory(kind)
      // Keep only allowlisted safe UI state, never backend messages/details.
      opened.value = result.ok && !result.error
      if (!opened.value) openError.value = clientError('diagnostics_directory_unavailable', new Error())
    } catch {
      openError.value = clientError('diagnostics_directory_unavailable', new Error())
    } finally {
      if (openError.value) directories.value = null
      opening.value = false
    }
  }
  return { data, directories, busy, error, opening, openError, opened, refresh, openDirectory }
})
