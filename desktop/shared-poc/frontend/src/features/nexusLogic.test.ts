import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import { ErrorCategory, type NexusMutationResult, type NexusSnapshotResult } from '../api/desktopApi'
import { nexusConnectionKey, nexusErrorKey, nexusOrigin, nexusPairingKey, useNexus } from './nexusLogic'
import type { desktopApi } from '../api/desktopApi'

function fixture(overrides: Partial<NexusSnapshotResult> = {}): NexusSnapshotResult {
  return {
    pairingState: 'not_paired',
    connectionState: 'unknown',
    generation: 'opaque-generation-1',
    deviceTokenStored: false,
    restartRequired: false,
    observedAt: '2026-10-08T06:30:00Z',
    capabilities: {
      canPair: true,
      canReconcile: false,
      pairDisabledReason: '',
      reconcileDisabledReason: 'nexus_not_paired',
    },
    ...overrides,
  }
}

function backend() {
  const nexusSnapshot = vi.fn<() => Promise<NexusSnapshotResult>>()
  const nexusPair = vi.fn()
  const nexusReconcile = vi.fn()
  return {
    nexusSnapshot,
    nexusPair,
    nexusReconcile,
    api: { nexusSnapshot, nexusPair, nexusReconcile } as unknown as typeof desktopApi,
  }
}

describe('nexusLogic', () => {
  it('keeps pairing distinct from Core connectivity', () => {
    expect(nexusPairingKey('paired')).toBe('nexus.paired')
    expect(nexusConnectionKey('unknown')).toBe('nexus.unknown')
    expect(nexusConnectionKey('disconnected')).toBe('nexus.disconnected')
    expect(nexusConnectionKey('connected')).toBe('nexus.connected')
    expect(nexusPairingKey('invalid')).toBe('nexus.invalid')
  })

  it('never renders URL path, credentials, query or fragments as a trusted origin', () => {
    expect(nexusOrigin('https://nexus.example.com/private/code')).toBe('https://nexus.example.com')
    expect(nexusOrigin('https://user:password@nexus.example.com/private')).toBe('')
    expect(nexusOrigin('https://nexus.example.com/?token=secret')).toBe('')
    expect(nexusOrigin('http://192.168.1.1/private')).toBe('')
    expect(nexusOrigin('http://localhost:3000/setup')).toBe('http://localhost:3000')
  })

  it('returns safe localized error keys for conflict, timeout and uncertain write', () => {
    expect(nexusErrorKey('nexus_generation_conflict')).toBe('nexus.conflict')
    expect(nexusErrorKey('nexus_pair_timeout')).toBe('nexus.outcome_unknown')
    expect(nexusErrorKey('nexus_identity_durability_unverified')).toBe('nexus.durability')
    expect(nexusErrorKey('unknown_backend_private/path?code=secret')).toBe('nexus.unavailable_help')
  })

  it('uses backend capabilities and never infers connected from paired status', async () => {
    const b = backend()
    b.nexusSnapshot.mockResolvedValue(fixture({
      pairingState: 'paired',
      connectionState: 'unknown',
      safeOrigin: 'https://nexus.example.com',
      deviceTokenStored: true,
      capabilities: { canPair: false, canReconcile: true, pairDisabledReason: 'unavailable', reconcileDisabledReason: '' },
    }))
    const state = useNexus(b.api)
    await state.refresh()
    expect(state.snapshot.value?.pairingState).toBe('paired')
    expect(state.snapshot.value?.connectionState).toBe('unknown')
    expect(state.canPair.value).toBe(false)
    expect(state.canReconcile.value).toBe(true)
    expect(b.nexusPair).not.toHaveBeenCalled()
    expect(b.nexusReconcile).not.toHaveBeenCalled()
    state.dispose()
  })

  it('re-reads stale generation without retrying the one-time pairing operation', async () => {
    const b = backend()
    b.nexusSnapshot
      .mockResolvedValueOnce(fixture())
      .mockResolvedValueOnce(fixture({ generation: 'opaque-generation-2' }))
    const state = useNexus(b.api)
    await state.refresh()
    const result: NexusMutationResult = {
      operationId: 'nexus-operation',
      completed: false,
      identitySaved: false,
      restartRequired: false,
      error: { code: 'nexus_generation_conflict', message: '', category: ErrorCategory.ErrorCategoryConflict, retryable: false },
    }
    await state.finishMutation(Promise.resolve(result))
    expect(state.notice.value).toBe('nexus.conflict')
    expect(state.snapshot.value?.generation).toBe('opaque-generation-2')
    expect(b.nexusSnapshot).toHaveBeenCalledTimes(2)
    expect(b.nexusPair).not.toHaveBeenCalled()
    state.dispose()
  })

  it('fails closed on unknown outcomes until an explicit refresh', async () => {
    const b = backend()
    b.nexusSnapshot.mockResolvedValue(fixture())
    const state = useNexus(b.api)
    await state.refresh()
    await state.finishMutation(Promise.reject(new Error('private-secret-path')))
    expect(state.notice.value).toBe('nexus.outcome_unknown')
    expect(state.snapshot.value).toBeNull()
    expect(state.canPair.value).toBe(false)
    expect(b.nexusPair).not.toHaveBeenCalled()
    expect(b.nexusSnapshot).toHaveBeenCalledTimes(1)
    await state.refresh()
    expect(state.canPair.value).toBe(true)
    state.dispose()
  })

  it('discards stale backend responses after unmount/disposal', async () => {
    let resolve!: (s: NexusSnapshotResult) => void
    const b = backend()
    b.nexusSnapshot.mockImplementation(() => new Promise<NexusSnapshotResult>((r) => { resolve = r }))
    const state = useNexus(b.api)
    const refresh = state.refresh()
    state.dispose()
    resolve(fixture())
    await refresh
    expect(state.snapshot.value).toBeNull()
  })

  it('keeps one-time code local to the form and clears it on mutation, cancel, timer and unmount', () => {
    const source = readFileSync(new URL('../components/connection/NexusPairForm.vue', import.meta.url), 'utf8')
    expect(source).toContain("const code = shallowRef('')")
    expect(source).toMatch(/function clearCode\(\).*code\.value = ''/)
    expect(source).toContain("clearCode()\n  approvedGeneration.value = ''")
    expect(source).toContain('onBeforeUnmount')
    expect(source).toContain('window.addEventListener(\'blur\', cancel)')
    for (const forbidden of ['localStorage', 'sessionStorage', 'console.log', 'console.error', 'pinia']) {
      expect(source).not.toContain(forbidden)
    }
  })
})
