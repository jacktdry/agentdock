import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { Domain, desktopApi } from '../api/desktopApi'
import type { Snapshot } from '../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import type { MessageKey } from '../i18n'
import { useContractStore } from './contract'

export const useBrowserStore = defineStore('browser', () => {
  const contract = useContractStore()
  const snapshot = ref<Snapshot | null>(null)
  const busy = ref(false)
  const error = ref<MessageKey | null>(null)
  const availabilityKey = computed<MessageKey>(() => {
    switch (snapshot.value?.availability) {
      case 'available': return 'browser.available'
      case 'browser_disabled': return 'browser.browser_disabled'
      case 'acp_disabled': return 'browser.acp_disabled'
      case 'broker_unavailable': return 'browser.broker_unavailable'
      default: return 'browser.core_unavailable'
    }
  })
  const stateKey = computed<MessageKey>(() => {
    if (snapshot.value?.availability !== 'available') return 'browser.unavailable'
    if (snapshot.value.stale) return 'browser.stale'
    switch (snapshot.value.state) {
      case 'idle': return 'browser.idle'
      case 'leases_present': return 'browser.leases_present'
      case 'stale': return 'browser.stale'
      default: return 'browser.unavailable'
    }
  })

  async function refresh() {
    if (busy.value) return
    busy.value = true
    snapshot.value = null
    error.value = null
    try {
      await contract.load()
      if (contract.error || !contract.canInvoke(Domain.DomainBrowser, 'snapshot')) {
        error.value = 'browser.capability_unavailable'
        return
      }
      const result = await desktopApi.browserSnapshot()
      if (result.error || result.snapshot.availability === 'core_unavailable') {
        error.value = 'browser.core_unavailable'
        return
      }
      snapshot.value = result.snapshot
    } catch {
      error.value = 'browser.snapshot_unavailable'
    } finally {
      busy.value = false
    }
  }
  return { snapshot, busy, error, availabilityKey, stateKey, refresh }
})
