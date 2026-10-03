import { defineStore } from 'pinia'
import { shallowRef } from 'vue'
import { clientError, desktopApi, type APIError, type UpdateStatus } from '../api/desktopApi'
export const useUpdateStore = defineStore('update', () => {
  const data = shallowRef<UpdateStatus | null>(null)
  const busy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  async function refresh() {
    if (busy.value) return
    busy.value = true; error.value = null
    try {
      const result = await desktopApi.checkUpdate()
      error.value = result.error ?? null
      data.value = error.value ? null : result.status
    } catch (caught) { data.value = null; error.value = clientError('update_call_failed', caught) }
    finally { busy.value = false }
  }
  return { data, busy, error, refresh }
})
