import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'
import {
  Availability,
  clientError,
  desktopApi,
  type APIError,
  type ConnectionActionName,
  type ConnectionSnapshot,
  type OAuthPasswordRevealResult,
  type OperationCapability,
  type PortObservation,
  type PublicEndpointResult,
} from '../api/desktopApi'
import { canPerformConnection } from '../features/connectionLogic'

export const useConnectionStore = defineStore('connection', () => {
  const snapshot = shallowRef<ConnectionSnapshot | null>(null)
  const preflight = shallowRef<PortObservation | null>(null)
  const publicEndpoint = shallowRef<PublicEndpointResult | null>(null)
  const busy = shallowRef(false)
  const preflightBusy = shallowRef(false)
  const endpointBusy = shallowRef(false)
  const revealBusy = shallowRef(false)
  const error = shallowRef<APIError | null>(null)
  const actionError = shallowRef<APIError | null>(null)
  const operationError = computed(() => actionError.value ?? error.value)
  const completed = shallowRef(false)
  const phase = shallowRef('')

  const publicEndpointState = computed(
    () => publicEndpoint.value?.state ?? snapshot.value?.tunnel.public_endpoint ?? 'unknown',
  )

  function capability(name: string): OperationCapability | undefined {
    return snapshot.value?.operations?.find((operation) => operation.name === name)
  }

  function canPerform(name: string) {
    return !busy.value && canPerformConnection(name, snapshot.value)
  }

  function disabledReason(name: string): string {
    const operation = capability(name)
    if (!operation || operation.availability !== Availability.AvailabilityAvailable) {
      return operation?.disabledReason ?? 'operation_unavailable'
    }
    return ''
  }

  function syncSnapshot(next: ConnectionSnapshot) {
    const previousRevision = snapshot.value?.configRevision
    snapshot.value = next
    if (previousRevision && previousRevision !== next.configRevision) {
      preflight.value = null
      publicEndpoint.value = null
    } else {
      if (preflight.value?.configRevision && preflight.value.configRevision !== next.configRevision) {
        preflight.value = null
      }
      if (publicEndpoint.value?.configRevision && publicEndpoint.value.configRevision !== next.configRevision) {
        publicEndpoint.value = null
      }
      if (
        publicEndpoint.value?.tunnelGeneration &&
        next.tunnelGeneration &&
        publicEndpoint.value.tunnelGeneration !== next.tunnelGeneration
      ) {
        publicEndpoint.value = null
      }
    }
  }

  function invalidateSnapshotObservations() {
    snapshot.value = null
    // A previous "reachable" endpoint or available port is not evidence of
    // current connectivity when the authoritative Core snapshot is unavailable.
    preflight.value = null
    publicEndpoint.value = null
  }

  async function readSnapshot(): Promise<boolean> {
    const result = await desktopApi.connectionSnapshot()
    error.value = result.error ?? null
    if (error.value || !result.snapshot) {
      invalidateSnapshotObservations()
      return false
    }
    syncSnapshot(result.snapshot)
    return true
  }

  async function refresh() {
    if (busy.value) return
    busy.value = true
    completed.value = false
    phase.value = ''
    error.value = null
    // Passive reads (including Overview/Connection page navigation) must not
    // erase the last mutation failure. Only a new mutation supersedes it.
    // The connection snapshot itself still fails closed on read errors.
    try {
      await readSnapshot()
    } catch (caught) {
      invalidateSnapshotObservations()
      error.value = clientError('connection_snapshot_call_failed', caught)
    } finally {
      busy.value = false
    }
  }

  async function preflightPort(candidatePort: number) {
    if (preflightBusy.value || !snapshot.value) return
    const revision = snapshot.value.configRevision
    preflightBusy.value = true
    error.value = null
    try {
      const result = await desktopApi.connectionPreflightPort(candidatePort)
      // A late result from a previous Core snapshot must never overwrite a
      // newer snapshot failure or resurrect its port observation.
      if (!snapshot.value || snapshot.value.configRevision !== revision) {
        preflight.value = null
        return
      }
      if (result.error) {
        error.value = result.error
        preflight.value = null
        return
      }
      if (result.observation.configRevision !== revision) {
        preflight.value = null
        return
      }
      preflight.value = result.observation
    } catch (caught) {
      preflight.value = null
      if (snapshot.value?.configRevision === revision) {
        error.value = clientError('connection_port_preflight_call_failed', caught)
      }
    } finally {
      preflightBusy.value = false
    }
  }

  async function runMutation(call: () => Promise<{ completed: boolean; phase: string; error?: APIError | null }>) {
    if (busy.value) return false
    busy.value = true
    completed.value = false
    phase.value = ''
    error.value = null
    actionError.value = null
    try {
      const result = await call()
      phase.value = result.phase
      actionError.value = result.error ?? (!result.completed ? clientError('connection_not_completed', '') : null)
      // An action acknowledgement cannot establish that the refreshed connection is healthy.
      try {
        const refreshed = await readSnapshot()
        completed.value = !actionError.value && refreshed
      } catch (caught) {
        invalidateSnapshotObservations()
        error.value = clientError('connection_snapshot_call_failed', caught)
      }
      return completed.value
    } catch (caught) {
      actionError.value = clientError('connection_mutation_call_failed', caught)
      try {
        await readSnapshot()
      } catch (refreshError) {
        invalidateSnapshotObservations()
        error.value = clientError('connection_snapshot_call_failed', refreshError)
      }
      return false
    } finally {
      busy.value = false
    }
  }

  async function perform(action: ConnectionActionName) {
    const current = snapshot.value
    if (!current || !canPerform(action)) return false
    return runMutation(() => desktopApi.connectionAction(action, current.configRevision))
  }

  async function updatePort(candidatePort: number) {
    const current = snapshot.value
    if (!current || !canPerform('updatePort')) return false
    return runMutation(() =>
      desktopApi.connectionUpdatePort({
        candidatePort,
        configRevision: current.configRevision,
      }),
    )
  }

  async function configureTunnel(mode: string, namedOrigin: string, newToken?: string) {
    const current = snapshot.value
    if (!current || !canPerform('configureTunnel')) return false
    return runMutation(() =>
      desktopApi.connectionConfigureTunnel({
        mode,
        namedOrigin,
        newToken,
        configRevision: current.configRevision,
      }),
    )
  }

  async function setTunnelAutostart(enabled: boolean) {
    const current = snapshot.value
    if (!current || !canPerform('setTunnelAutostart')) return false
    return runMutation(() =>
      desktopApi.connectionSetTunnelAutostart({
        enabled,
        configRevision: current.configRevision,
      }),
    )
  }

  async function testPublicEndpoint() {
    const current = snapshot.value
    if (!current || endpointBusy.value || !canPerform('testPublicEndpoint')) return
    endpointBusy.value = true
    error.value = null
    try {
      const result = await desktopApi.connectionTestPublicEndpoint()
      const latest = snapshot.value
      if (!latest) return
      if (result.configRevision && result.configRevision !== latest.configRevision) return
      if (result.tunnelGeneration && latest.tunnelGeneration && result.tunnelGeneration !== latest.tunnelGeneration) return
      if (result.error) {
        error.value = result.error
        publicEndpoint.value = result
        return
      }
      publicEndpoint.value = result
    } catch (caught) {
      error.value = clientError('connection_endpoint_test_call_failed', caught)
    } finally {
      endpointBusy.value = false
    }
  }

  async function revealOAuthPassword(): Promise<OAuthPasswordRevealResult | null> {
    const current = snapshot.value
    if (!current || revealBusy.value || !canPerform('revealOAuthPassword')) return null
    const revision = current.configRevision
    const generation = current.tunnelGeneration
    revealBusy.value = true
    error.value = null
    try {
      const result = await desktopApi.connectionRevealOAuthPassword()
      if (result.error) {
        error.value = result.error
        return null
      }
      if (result.configRevision && result.configRevision !== revision) return null
      if (result.tunnelGeneration && generation && result.tunnelGeneration !== generation) return null
      if (snapshot.value?.configRevision !== revision) return null
      return result
    } catch (caught) {
      error.value = clientError('connection_password_reveal_call_failed', caught)
      return null
    } finally {
      revealBusy.value = false
    }
  }

  function clearPreflight() {
    preflight.value = null
  }

  return {
    snapshot,
    preflight,
    publicEndpoint,
    publicEndpointState,
    busy,
    preflightBusy,
    endpointBusy,
    revealBusy,
    error,
    actionError,
    operationError,
    completed,
    phase,
    capability,
    canPerform,
    disabledReason,
    refresh,
    preflightPort,
    perform,
    updatePort,
    configureTunnel,
    setTunnelAutostart,
    testPublicEndpoint,
    revealOAuthPassword,
    clearPreflight,
  }
})
