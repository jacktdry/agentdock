import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ExecutionInsertion, ExecutionSnapshot, ExecutionStreamEnvelope } from '../api/executionContract'
import { useExecutionStore } from './execution'

const api = vi.hoisted(() => ({
  openExecutionStream: vi.fn(), enqueueInsertion: vi.fn(), cancelInsertion: vi.fn(), close: vi.fn(),
}))
vi.mock('../api/desktopApi', () => ({
  desktopApi: api,
  clientError: (code: string) => ({ code, category: 'internal', message: '', retryable: false }),
}))

const snapshot: ExecutionSnapshot = {
  schemaVersion: 1, epoch: 'epoch', latestSequence: '1', prunedThrough: '0', activeCalls: 1,
  calls: [{ callID: 'root', parentCallID: '', tool: 'command', source: 'core', status: 'running', insertionSupported: true, startedSequence: '1', endedSequence: '', startedAt: 'start', updatedAt: 'start', completedAt: '', errorCode: '', errorCategory: '', continuationID: '' }], insertions: [],
}
const insertion: ExecutionInsertion = {
  insertionID: 'ins', targetCallID: 'root', status: 'accepted', textPreview: 'private', textBytes: 5,
  createdAt: 'created', updatedAt: 'created', expiresAt: 'expires', deliveredAt: '', reason: '',
}
const envelope = (value = snapshot): ExecutionStreamEnvelope => ({
  kind: 'snapshot', schemaVersion: 1, snapshot: value, page: null, id: '', code: '', reconnectTotal: '2', transportDroppedTotal: '3',
})
const statusEnvelope = (code: string): ExecutionStreamEnvelope => ({
  ...envelope(), kind: 'status', snapshot: null, code,
})
let receive: (message: ExecutionStreamEnvelope) => void
let streamStatus: (status: 'open' | 'closed' | 'error') => void

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  api.openExecutionStream.mockImplementation((onMessage, onState) => {
    receive = onMessage
    streamStatus = onState
    onState('open')
    return { ready: Promise.resolve(), close: api.close }
  })
})

describe('execution store integration', () => {
  it('requires a snapshot before enabling controls, replaces later snapshots and keeps disconnect separate from call state', async () => {
    const store = useExecutionStore()
    await store.connect()
    expect(store.canControl).toBe(false)
    receive(envelope())
    expect(store.canControl).toBe(false)
    receive(statusEnvelope('execution_synchronized'))
    expect(store.canControl).toBe(true)
    expect([store.reconnectTotal, store.transportDroppedTotal]).toEqual(['2', '3'])
    streamStatus('error')
    expect(store.state.calls[0].status).toBe('running')
    expect(store.canControl).toBe(false)
    streamStatus('open')
    receive(envelope({ ...snapshot, epoch: 'restart', calls: [] }))
    expect(store.canControl).toBe(false)
    receive(statusEnvelope('execution_synchronized'))
    expect(store.state.calls).toEqual([])
    expect(store.state.epoch).toBe('restart')
    store.disconnect()
    expect(api.close).toHaveBeenCalledOnce()
    expect(store.streamState).toBe('closed')
  })

  it('returns success only for accepted ACK, updates immediately and reconciles streamed delivery', async () => {
    const store = useExecutionStore()
    await store.connect()
    receive(envelope())
    receive(statusEnvelope('execution_synchronized'))
    api.enqueueInsertion.mockResolvedValue({ ack: true, error: null, insertion })
    expect(await store.enqueue('root', ' hello ')).toBe(true)
    expect(api.enqueueInsertion).toHaveBeenCalledWith('root', 'hello')
    expect(store.state.insertions[0].status).toBe('accepted')
    expect(store.state.insertions[0]).not.toHaveProperty('textPreview')
    receive({ ...envelope(), kind: 'activity', snapshot: null, page: {
      schemaVersion: 1, epoch: 'epoch', after: '1', latestSequence: '2', prunedThrough: '0', gap: false, hasMore: false,
      events: [{ schemaVersion: 1, epoch: 'epoch', sequence: '2', occurredAt: 'now', kind: 'insertion.delivered', callID: 'root', parentCallID: '', tool: '', source: '', status: '', insertionSupported: false, errorCode: '', errorCategory: '', insertionID: 'ins', output: null, fileChange: null }],
    } })
    expect(store.lastInsertionStatus).toBe('delivered')
    await store.cancel('ins')
    expect(api.cancelInsertion).not.toHaveBeenCalled()
    api.enqueueInsertion.mockResolvedValue({ ack: false, error: null, insertion: { ...insertion, insertionID: 'second' } })
    expect(await store.enqueue('root', 'hello')).toBe(false)
    expect(store.controlError?.code).toBe('insertion_not_accepted')
    store.disconnect()
  })

  it('blocks invalid/terminal targets and oversized UTF-8 input before sending, and cancels only accepted insertions', async () => {
    const store = useExecutionStore()
    await store.connect()
    receive(envelope({ ...snapshot, insertions: [insertion] }))
    receive(statusEnvelope('execution_synchronized'))
    expect(await store.enqueue('missing', 'hello')).toBe(false)
    expect(await store.enqueue('root', '界'.repeat(2731))).toBe(false)
    expect(api.enqueueInsertion).not.toHaveBeenCalled()
    api.cancelInsertion.mockResolvedValue({ error: null, ack: false, insertion: { ...insertion, status: 'cancelled', reason: 'cancelled_by_user' } })
    await store.cancel('ins')
    expect(store.state.insertions[0]).toMatchObject({ status: 'cancelled', reason: 'cancelled_by_user' })
    await store.cancel('ins')
    expect(api.cancelInsertion).toHaveBeenCalledOnce()
    receive(envelope({ ...snapshot, calls: snapshot.calls.map(call => ({ ...call, status: 'completed' })) }))
    expect(await store.enqueue('root', 'hello')).toBe(false)
    store.disconnect()
  })

  it('keeps same-epoch replay visible while reconnecting but disables controls until Core reconnects', async () => {
    const store = useExecutionStore()
    await store.connect()
    receive(envelope())
    receive(statusEnvelope('execution_synchronized'))
    expect(store.canControl).toBe(true)

    receive(statusEnvelope('execution_reconnecting'))
    expect(store.synchronized).toBe(false)
    expect(store.canControl).toBe(false)

    receive({ ...envelope(), kind: 'activity', snapshot: null, page: {
      schemaVersion: 1, epoch: 'epoch', after: '1', latestSequence: '2', prunedThrough: '0', gap: false, hasMore: false,
      events: [{ schemaVersion: 1, epoch: 'epoch', sequence: '2', occurredAt: 'now', kind: 'call.waiting', callID: 'root', parentCallID: '', tool: '', source: '', status: 'waiting_for_user', insertionSupported: true, errorCode: '', errorCategory: '', insertionID: '', output: null, fileChange: null }],
    } })
    expect(store.state.calls[0].status).toBe('waiting_for_user')
    expect(store.canControl).toBe(false)

    receive(statusEnvelope('execution_synchronized'))
    expect(store.synchronized).toBe(true)
    expect(store.canControl).toBe(true)
    store.disconnect()
  })

  it('does not resurrect state from a stale ACK after an epoch reset', async () => {
    const store = useExecutionStore()
    await store.connect()
    receive(envelope())
    receive(statusEnvelope('execution_synchronized'))
    let finish!: (value: unknown) => void
    api.enqueueInsertion.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const result = store.enqueue('root', 'hello')
    receive(envelope({ ...snapshot, epoch: 'restart', calls: [] }))
    finish({ ack: true, error: null, insertion })
    expect(await result).toBe(false)
    expect(store.state.insertions).toEqual([])
    expect(store.controlError?.code).toBe('execution_resync_required')
    store.disconnect()
  })
})
