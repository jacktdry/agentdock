import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as ACPService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/acpservice'
import type { ACPLifecycleUpdate, ACPMutationResult, ACPStatusResult, APIError } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { Domain, clientError } from '../api/desktopApi'
import { useContractStore } from './contract'

export const useACPStore = defineStore('acp', () => {
  const contract = useContractStore()
  const status = ref<ACPStatusResult | null>(null)
  const loading = ref(false)
  const pending = ref(false)
  const error = ref<APIError | null>(null)
  const completed = ref(false)
  const busy = computed(() => loading.value || pending.value)
  const canRead = computed(() => contract.canInvoke(Domain.DomainACP, 'status'))
  const state = computed(() => loading.value ? 'loading' : !canRead.value || error.value || !status.value ? 'unavailable' : !status.value.enabled ? 'disabled' : !(status.value.profiles?.length) ? 'empty' : 'ready')
  function canInvoke(operation: 'close' | 'updateLifecycle') {
    return !busy.value && !!status.value?.enabled && contract.canInvoke(Domain.DomainACP, operation)
  }
  async function readStatus() {
    loading.value = true
    try {
      const result = await ACPService.Status()
      error.value = result.error ?? null
      if (!error.value) status.value = result
    } catch {
      error.value = clientError('acp_status_failed', '')
    } finally { loading.value = false }
  }
  async function refresh() {
    if (busy.value || !canRead.value) return
    error.value = null
    completed.value = false
    await readStatus()
  }
  async function mutate(operation: 'close' | 'updateLifecycle', call: () => Promise<ACPMutationResult>) {
    if (!canInvoke(operation)) return
    pending.value = true
    error.value = null
    completed.value = false
    try {
      const result = await call()
      error.value = result.error ?? (result.completed ? null : clientError('acp_mutation_incomplete', ''))
      if (!error.value) {
        completed.value = true
        if (canRead.value) await readStatus()
      }
    } catch {
      error.value = clientError('acp_mutation_failed', '')
    } finally { pending.value = false }
  }
  const close = (profileId: string, sessionId: string) => mutate('close', () => ACPService.Close(profileId, sessionId))
  const updateLifecycle = (update: ACPLifecycleUpdate) => mutate('updateLifecycle', () => ACPService.UpdateLifecycle(update))
  return { status, loading, error, completed, busy, canRead, state, canInvoke, refresh, close, updateLifecycle }
})
