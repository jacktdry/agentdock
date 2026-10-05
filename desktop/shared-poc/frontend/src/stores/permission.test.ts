import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
const mocks = vi.hoisted(() => ({ status: vi.fn(), history: vi.fn(), begin: vi.fn(), once: vi.fn(), workspace: vi.fn(), reject: vi.fn(), update: vi.fn() }))
vi.mock('../../bindings/github.com/uvwt/agentdock/internal/desktopapi/permissionservice', () => ({
  Status: mocks.status, History: mocks.history, BeginConfirmation: mocks.begin,
  ApproveOnce: mocks.once, ApproveWorkspace: mocks.workspace, Reject: mocks.reject, UpdatePolicy: mocks.update,
}))
vi.mock('./contract', () => ({ useContractStore: () => ({ canInvoke: () => true }) }))
vi.mock('../api/desktopApi', () => ({ Domain: { DomainPermission: 'permission' } }))
import { usePermissionStore } from './permission'
import type { ApprovalRecord } from '../../bindings/github.com/uvwt/agentdock/internal/permission/models'
const policy = { schema_version: 1, revision: 1, global_mode: 'rules', settings: { permission_profile: { filesystem: 'write', network: 'allow', sandbox_boundary: 'none' }, approval_policy: { mode: 'on-request' }, approval_reviewer: 'defer' }, scopes: [], rules: [] }
const approval = { approval_id: 'a1', version: 1, status: 'pending', can_approve_once: true, can_approve_workspace: false, workspace_unavailable_reason: 'Opaque effects' } as ApprovalRecord
beforeEach(() => {
  vi.resetAllMocks()
  setActivePinia(createPinia())
  mocks.status.mockResolvedValue({ ok: true, schema_version: 1, state_revision: 1, runtime_epoch: 'epoch', policy })
  mocks.history.mockResolvedValue({ ok: true, schema_version: 1, state_revision: 1, runtime_epoch: 'epoch', approvals: [approval] })
  mocks.begin.mockResolvedValue({ ok: true, confirmation: { confirmation_id: 'c1', expires_at: new Date(Date.now() + 60000).toISOString() } })
  mocks.once.mockResolvedValue({ ok: true })
})
describe('Permission Core truth', () => {
  it('restores pending approvals on reconnect and refuses incoherent revisions', async () => {
    const store = usePermissionStore()
    await store.refresh()
    expect(store.pending).toEqual([approval])
    await store.begin('approve_once', approval)
    mocks.history.mockResolvedValue({ ok: true, state_revision: 2, runtime_epoch: 'epoch', approvals: [] })
    await store.refresh()
    expect(store.fresh).toBe(false)
    expect(store.canMutate).toBe(false)
    expect(store.confirmation).toBeNull()
    mocks.status.mockResolvedValue({ ok: true, state_revision: 2, runtime_epoch: 'epoch', policy })
    await store.refresh()
    expect(store.pending).toEqual([])
    expect(store.fresh).toBe(true)
  })
  it('truthfully refuses unavailable workspace approval without a challenge or mutation', async () => {
    const store = usePermissionStore()
    await store.refresh()
    await store.begin('approve_workspace', approval)
    expect(mocks.begin).not.toHaveBeenCalled()
    expect(mocks.workspace).not.toHaveBeenCalled()
  })
  it('requires explicit second confirmation, sends exact bindings and never replays', async () => {
    const store = usePermissionStore()
    await store.refresh()
    await store.begin('approve_once', approval)
    expect(mocks.once).not.toHaveBeenCalled()
    await Promise.all([store.confirm(), store.confirm()])
    expect(mocks.once).toHaveBeenCalledExactlyOnceWith('c1', 'a1', 1, 1)
    expect(mocks.history).toHaveBeenCalledTimes(2)
    expect(store.completed).toBe(true)
    expect(store.pending[0]?.status).toBe('pending') // No optimistic approval or operation execution.
  })
  it.each(['POLICY_REVISION_CONFLICT', 'APPROVAL_VERSION_CONFLICT', 'PERMISSION_CONFIRMATION_NOT_FOUND', 'PERMISSION_CONFIRMATION_EXPIRED', 'PERMISSION_CONFIRMATION_MISMATCH', 'APPROVAL_EXPIRED', 'APPROVAL_NOT_ELIGIBLE'])('refreshes after %s and never auto retries', async code => {
    const store = usePermissionStore()
    await store.refresh()
    await store.begin('approve_once', approval)
    mocks.once.mockResolvedValue({ ok: false, error: { code } })
    mocks.history.mockResolvedValue({ ok: true, state_revision: 1, runtime_epoch: 'epoch', approvals: [{ ...approval, status: 'invalidated' }] })
    await store.confirm()
    expect(store.pending).toEqual([])
    expect(store.confirmation).toBeNull()
    expect(store.error).toBe('PERMISSION_DECISION_FAILED')
    expect(mocks.once).toHaveBeenCalledTimes(1)
  })
  it('refuses local challenge expiry and sanitizes thrown credential errors', async () => {
    const store = usePermissionStore()
    await store.refresh()
    mocks.begin.mockResolvedValue({ ok: true, confirmation: { confirmation_id: 'expired', expires_at: new Date(0).toISOString() } })
    await store.begin('approve_once', approval)
    await store.confirm()
    expect(mocks.once).not.toHaveBeenCalled()
    mocks.status.mockRejectedValue(new Error('Bearer desktop-secret'))
    await store.refresh()
    expect(store.error).not.toContain('secret')
    expect(store.canMutate).toBe(false)
  })
  it('keeps policy payload detached from editable state until explicit confirmation', async () => {
    const store = usePermissionStore()
    await store.refresh()
    const draft = structuredClone(policy)
    await store.begin('update_policy', undefined, draft)
    draft.settings.approval_policy.mode = 'never'
    mocks.update.mockResolvedValue({ ok: true })
    await store.confirm()
    expect(mocks.update.mock.calls[0]?.[2].settings.approval_policy.mode).toBe('on-request')
  })
})
