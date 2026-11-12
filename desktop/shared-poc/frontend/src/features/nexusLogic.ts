import { computed, shallowRef } from 'vue'
import { desktopApi, type NexusMutationResult, type NexusSnapshotResult } from '../api/desktopApi'
import type { MessageKey } from '../i18n'

// Display only the origin; endpoint policy and destination validation belong to Go.
export function nexusOrigin(value: string): string {
  try {
    if (value.length > 4096 || /[\s\\]/.test(value)) return ''
    const url = new URL(value)
    if (url.username || url.password || url.search || url.hash || value.includes('?') || value.includes('#')) return ''
    if (url.protocol !== 'https:' && !(url.protocol === 'http:' &&
      (url.hostname === 'localhost' || url.hostname === '[::1]' || /^127\.(\d{1,3}\.){2}\d{1,3}$/.test(url.hostname)))) return ''
    return url.origin
  } catch { return '' }
}

export function nexusPairingKey(state?: string): MessageKey {
  return ({ not_paired: 'nexus.not_paired', paired: 'nexus.paired', invalid: 'nexus.invalid' } as const)[state as 'not_paired'] ?? 'nexus.unknown'
}

export function nexusConnectionKey(state?: string): MessageKey {
  return ({ connected: 'nexus.connected', disconnected: 'nexus.disconnected', unavailable: 'nexus.unavailable', checking: 'nexus.checking' } as const)[state as 'connected'] ?? 'nexus.unknown'
}

export function nexusErrorKey(code?: string): MessageKey {
  return ({
    nexus_generation_conflict: 'nexus.conflict',
    nexus_identity_changed: 'nexus.conflict',
    nexus_replace_confirmation_required: 'nexus.replace_required',
    nexus_identity_invalid: 'nexus.invalid',
    nexus_pair_denied: 'nexus.denied',
    nexus_endpoint_invalid: 'nexus.endpoint_invalid',
    nexus_endpoint_blocked: 'nexus.endpoint_invalid',
    nexus_not_paired: 'nexus.not_paired',
    nexus_restart_required: 'nexus.restart_required',
    nexus_identity_durability_unverified: 'nexus.durability',
    nexus_pair_timeout: 'nexus.outcome_unknown',
    nexus_outcome_unknown: 'nexus.outcome_unknown',
    nexus_mutation_busy: 'nexus.busy',
  } as Record<string, MessageKey>)[code ?? ''] ?? 'nexus.unavailable_help'
}

// Instance-local observation state. No Pinia, storage, draft, or pairing code.
export function useNexus(api = desktopApi) {
  const snapshot = shallowRef<NexusSnapshotResult | null>(null)
  const busy = shallowRef(false)
  const notice = shallowRef<MessageKey | null>(null)
  const result = shallowRef<Pick<NexusMutationResult, 'completed' | 'identitySaved' | 'restartRequired'> | null>(null)
  let active = true
  const canPair = computed(() => !busy.value && !!snapshot.value?.generation &&
    snapshot.value.capabilities.canPair && ['not_paired', 'paired'].includes(snapshot.value.pairingState))
  const canReconcile = computed(() => !busy.value && !!snapshot.value?.generation &&
    snapshot.value.capabilities.canReconcile && snapshot.value.pairingState === 'paired')

  async function observe() {
    snapshot.value = null // Never allow an old generation after a failed observation.
    try {
      const next = await api.nexusSnapshot()
      if (active) snapshot.value = next
    } catch {
      if (active) notice.value = 'nexus.unavailable_help'
    }
  }
  async function refresh() {
    if (busy.value || !active) return
    busy.value = true
    notice.value = null
    await observe()
    if (active) busy.value = false
  }
  async function finishMutation(pending: Promise<NexusMutationResult>) {
    busy.value = true
    notice.value = null
    result.value = null
    try {
      const next = await pending
      if (!active) return
      result.value = { completed: next.completed, identitySaved: next.identitySaved, restartRequired: next.restartRequired }
      notice.value = next.error ? nexusErrorKey(next.error.code) :
        next.completed ? 'nexus.applied' : 'nexus.outcome_unknown'
      snapshot.value = null
      if (!next.error || !['nexus_outcome_unknown', 'nexus_pair_timeout'].includes(next.error.code)) await observe()
    } catch {
      if (active) { snapshot.value = null; notice.value = 'nexus.outcome_unknown' }
    } finally {
      if (active) busy.value = false
    }
  }
  function dispose() { active = false; snapshot.value = null }
  return { snapshot, busy, notice, result, canPair, canReconcile, refresh, finishMutation, dispose }
}
