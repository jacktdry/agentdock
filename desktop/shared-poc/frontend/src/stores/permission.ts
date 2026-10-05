import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'
import * as service from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/permissionservice'
import type { PermissionResult } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type { ApprovalRecord, ConfirmationChallenge, ControlMutationRequest, Policy } from '../../bindings/github.com/uvwt/agentdock/internal/permission/models'
import { Domain } from '../api/desktopApi'
import { useContractStore } from './contract'

export const usePermissionStore = defineStore('permission', () => {
  const contract = useContractStore()
  const snapshot = shallowRef<PermissionResult | null>(null)
  const history = shallowRef<ApprovalRecord[]>([])
  const busy = ref(false)
  const fresh = ref(false)
  const error = ref('')
  const completed = ref(false)
  const confirmation = shallowRef<{ mutation: ControlMutationRequest; challenge: ConfirmationChallenge; approval?: ApprovalRecord } | null>(null)
  const pending = computed(() => history.value.filter(record => record.status === 'pending'))
  const canInvoke = (operation: string) => contract.canInvoke(Domain.DomainPermission, operation)
  const canMutate = computed(() => fresh.value && !busy.value && !confirmation.value && canInvoke('beginConfirmation'))
  let confirmationGeneration = 0
  function cancel() { confirmationGeneration++; confirmation.value = null }
  function check(result: PermissionResult) {
    if (!result.ok || result.error) throw new Error(result.error?.code ?? 'PERMISSION_RESPONSE_INVALID')
    return result
  }
  async function readTruth() {
    fresh.value = false
    if (!canInvoke('status') || !canInvoke('history')) throw new Error('PERMISSION_UNAVAILABLE')
    // Compare Core revisions across both reads; never combine different epochs.
    const state = check(await service.Status())
    const records = check(await service.History('', 512))
    if (!state.policy || state.state_revision !== records.state_revision || state.runtime_epoch !== records.runtime_epoch) {
      throw new Error('PERMISSION_STATE_CHANGED')
    }
    snapshot.value = state
    history.value = records.approvals ?? []
    fresh.value = true
  }
  async function refresh() {
    if (busy.value) return
    busy.value = true
    cancel()
    error.value = ''
    try { await readTruth() } catch { error.value = 'PERMISSION_REFRESH_REQUIRED' }
    finally { busy.value = false }
  }
  async function begin(kind: string, approval?: ApprovalRecord, policy?: Policy) {
    if (!canMutate.value || !snapshot.value?.policy) return
    const operation = { update_policy: 'updatePolicy', approve_once: 'approveOnce', approve_workspace: 'approveWorkspace', reject: 'reject' }[kind]
    if (!operation || !canInvoke(operation)) return
    if (approval && (approval.status !== 'pending' || (kind === 'approve_once' && !approval.can_approve_once) || (kind === 'approve_workspace' && !approval.can_approve_workspace))) return
    const mutation: ControlMutationRequest = {
      kind, policy_revision: snapshot.value.policy.revision,
      ...(approval ? { approval_id: approval.approval_id, approval_version: approval.version } : {}),
      ...(policy ? { policy: JSON.parse(JSON.stringify(policy)) } : {}),
    }
    const generation = confirmationGeneration
    busy.value = true
    completed.value = false
    error.value = ''
    try {
      const result = check(await service.BeginConfirmation(mutation))
      if (!result.confirmation) throw new Error('INVALID_CONFIRMATION')
      if (generation === confirmationGeneration) confirmation.value = { mutation, challenge: result.confirmation, ...(approval ? { approval: JSON.parse(JSON.stringify(approval)) } : {}) }
    } catch {
      error.value = 'PERMISSION_CONFIRMATION_FAILED'
      try { await readTruth() } catch { fresh.value = false }
    } finally { busy.value = false }
  }
  async function confirm() {
    if (busy.value || !confirmation.value) return
    const { mutation, challenge } = confirmation.value
    cancel() // Remove before awaiting: no double click or replay.
    busy.value = true
    fresh.value = false
    error.value = ''
    try {
      if (!Number.isFinite(Date.parse(challenge.expires_at)) || Date.parse(challenge.expires_at) <= Date.now()) throw new Error('EXPIRED')
      const id = challenge.confirmation_id
      const args = [id, mutation.approval_id ?? '', mutation.approval_version ?? 0, mutation.policy_revision] as const
      let result: PermissionResult
      switch (mutation.kind) {
        case 'update_policy': result = await service.UpdatePolicy(id, mutation.policy_revision, mutation.policy!); break
        case 'approve_once': result = await service.ApproveOnce(...args); break
        case 'approve_workspace': result = await service.ApproveWorkspace(...args); break
        case 'reject': result = await service.Reject(...args); break
        default: throw new Error('INVALID_MUTATION')
      }
      check(result)
      completed.value = true
    } catch { error.value = 'PERMISSION_DECISION_FAILED' }
    finally {
      // Re-read even on conflict, consumed/expired/invalidation or lost response.
      try { await readTruth() } catch { error.value = 'PERMISSION_REFRESH_REQUIRED'; fresh.value = false }
      busy.value = false
    }
  }
  return { snapshot, history, pending, busy, fresh, error, completed, confirmation, canMutate, canInvoke, refresh, begin, confirm, cancel }
})
