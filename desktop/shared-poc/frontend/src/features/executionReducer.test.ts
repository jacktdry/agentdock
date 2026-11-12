import { describe, expect, it } from 'vitest'
import { MAX_EXECUTION_EVENTS, MAX_VISIBLE_CALLS, type ExecutionCall, type ExecutionEvent, type ExecutionInsertion, type ExecutionPage, type ExecutionSnapshot } from '../api/executionContract'
import { emptyExecution, executionRows, isActiveCall, MAX_VISIBLE_INSERTIONS, reconcileInsertion, reduceExecutionPage, replaceExecution, validInsertionText } from './executionReducer'

export const call = (callID = 'root', extra: Partial<ExecutionCall> = {}): ExecutionCall => ({
  callID, parentCallID: '', tool: 'command', source: 'core', status: 'running', insertionSupported: true, startedSequence: '1',
  endedSequence: '', startedAt: 'start', updatedAt: 'update', completedAt: '', errorCode: '', errorCategory: '', continuationID: '', ...extra,
})
export const insertion = (extra: Partial<ExecutionInsertion> = {}): ExecutionInsertion => ({
  insertionID: 'ins', targetCallID: 'root', status: 'accepted', textPreview: 'private user text', textBytes: 17,
  createdAt: 'created', updatedAt: 'updated', expiresAt: 'expires', deliveredAt: '', reason: '', ...extra,
})
export const snapshot = (extra: Partial<ExecutionSnapshot> = {}): ExecutionSnapshot => ({
  schemaVersion: 1, epoch: 'epoch', latestSequence: '1', prunedThrough: '0', activeCalls: 1, calls: [call()], insertions: [], ...extra,
})
export const event = (sequence: string, kind: string, extra: Partial<ExecutionEvent> = {}): ExecutionEvent => ({
  schemaVersion: 1, epoch: 'epoch', sequence, occurredAt: 'event-time', kind, callID: 'root', parentCallID: '', tool: '', source: '', status: '',
  insertionSupported: false, errorCode: '', errorCategory: '', insertionID: '', output: null, fileChange: null, ...extra,
})
export const page = (events: ExecutionEvent[], extra: Partial<ExecutionPage> = {}): ExecutionPage => ({
  schemaVersion: 1, epoch: 'epoch', after: '1', latestSequence: events.at(-1)?.sequence ?? '1', prunedThrough: '0', gap: false, hasMore: false, events, ...extra,
})

describe('authoritative execution reducer', () => {
  it('replaces initial and later same-epoch snapshots rather than merging stale calls or events', () => {
    const prior = reduceExecutionPage(replaceExecution(snapshot({ insertions: [insertion()] })), page([event('2', 'call.waiting')]))
    const next = replaceExecution(snapshot({ calls: [call('replacement')], latestSequence: '9' }))
    expect(next.calls.map(item => item.callID)).toEqual(['replacement'])
    expect(next.insertions).toEqual([])
    expect(next.events).toEqual([])
    expect(next.cursor).toBe('9')
    expect(prior.calls[0].status).toBe('waiting_for_user')
  })

  it('resets epoch and discards incompatible history', () => {
    const next = replaceExecution(snapshot({ epoch: 'restart', latestSequence: '0', calls: [], insertions: [] }))
    expect(next).toEqual({ ...emptyExecution(), epoch: 'restart' })
  })

  it('uses kind for lifecycle, preserves root/child facts and explicit waiting state', () => {
    let state = replaceExecution(snapshot())
    state = reduceExecutionPage(state, page([event('2', 'call.started', { callID: 'child', parentCallID: 'root', tool: 'mcp', source: 'remote' }), event('3', 'call.waiting', { callID: 'child', status: 'running' })]))
    expect(executionRows(state.calls, true).map(row => [row.call.callID, row.depth])).toEqual([['root', 0], ['child', 1]])
    expect(state.calls.find(item => item.callID === 'child')?.status).toBe('waiting_for_user')
    expect(state.calls.every(isActiveCall)).toBe(true)
    state = reduceExecutionPage(state, page([event('4', 'call.resumed', { callID: 'child' }), event('5', 'call.completed', { callID: 'root' }), event('6', 'call.failed', { callID: 'child', errorCode: 'tool_failed', errorCategory: 'execution' })]))
    const child = state.calls.find(item => item.callID === 'child')!
    expect(child).toMatchObject({ parentCallID: 'root', tool: 'mcp', source: 'remote', status: 'failed', endedSequence: '6', errorCode: 'tool_failed' })
    state = reduceExecutionPage(state, page([event('7', 'call.started', { callID: 'another' }), event('8', 'call.cancelled', { callID: 'another' })]))
    expect(state.calls.find(item => item.callID === 'another')?.status).toBe('cancelled')
    expect(state.calls.some(isActiveCall)).toBe(false)
  })

  it('never fabricates a pruned parent and keeps the child association', () => {
    const state = replaceExecution(snapshot({ calls: [call('child', { parentCallID: 'pruned' })] }))
    expect(executionRows(state.calls, true)).toEqual([{ call: state.calls[0], depth: 0 }])
    expect(state.calls).toHaveLength(1)
    expect(state.calls[0].parentCallID).toBe('pruned')
  })

  it.each(['delivered', 'rejected', 'expired', 'cancelled'] as const)('reconciles %s insertions without inventing reason, text or timestamps', status => {
    let state = reduceExecutionPage(replaceExecution(snapshot()), page([event('2', 'insertion.accepted', { insertionID: 'ins', errorCode: 'call_error' })]))
    expect(state.insertions[0]).toMatchObject({ insertionID: 'ins', targetCallID: 'root', status: 'accepted', textBytes: null, createdAt: '', updatedAt: '', expiresAt: '', reason: '' })
    expect(state.insertions[0]).not.toHaveProperty('textPreview')
    state = reduceExecutionPage(state, page([event('3', `insertion.${status}`, { insertionID: 'ins', errorCode: 'never_an_insertion_reason' })]))
    expect(state.insertions[0].status).toBe(status)
    expect(state.insertions[0].reason).toBe('')
  })

  it('preserves authoritative reasons and does not regress terminal state with a delayed accepted ACK/event', () => {
    let state = replaceExecution(snapshot({ insertions: [insertion({ status: 'cancelled', reason: 'cancelled_by_user' })] }))
    state = reduceExecutionPage(state, page([event('2', 'insertion.accepted', { insertionID: 'ins' })]))
    state = reconcileInsertion(state, insertion())
    expect(state.insertions[0]).toMatchObject({ status: 'cancelled', reason: 'cancelled_by_user' })
    state = reduceExecutionPage(state, page([event('3', 'insertion.cancelled', { insertionID: 'ins', errorCode: 'call_error' })]))
    expect(state.insertions[0].reason).toBe('cancelled_by_user')
  })

  it('bounds snapshots, events, calls and insertion history while prioritizing active calls', () => {
    let state = replaceExecution(snapshot({ calls: [...Array.from({ length: 200 }, (_, i) => call(`old${i}`, { status: 'completed', endedSequence: String(i + 2) })), call('active')] }))
    expect(state.calls).toHaveLength(MAX_VISIBLE_CALLS)
    expect(state.calls[0].callID).toBe('active')
    state = reduceExecutionPage(state, page(Array.from({ length: 500 }, (_, i) => event(String(i + 2), 'call.started', { callID: `new${i}` }))))
    expect(state.calls).toHaveLength(MAX_VISIBLE_CALLS)
    expect(state.events).toHaveLength(MAX_EXECUTION_EVENTS)
    expect(state.events[0].sequence).toBe('302')
    for (let i = 0; i < 500; i++) state = reconcileInsertion(state, insertion({ insertionID: `ins${i}`, status: i === 0 ? 'accepted' : 'delivered' }))
    expect(state.insertions).toHaveLength(MAX_VISIBLE_INSERTIONS)
    expect(state.insertions.some(item => item.insertionID === 'ins0')).toBe(true)
  })

  it('ignores wrong epochs, gap pages and duplicate events; cursor precision survives partial replay', () => {
    const prior = replaceExecution(snapshot({ latestSequence: '9007199254740992' }))
    expect(reduceExecutionPage(prior, page([], { epoch: 'other' }))).toBe(prior)
    expect(reduceExecutionPage(prior, page([], { gap: true }))).toBe(prior)
    const next = reduceExecutionPage(prior, page([
      event('9007199254740993', 'call.waiting'), event('9007199254740993', 'call.resumed'),
      event('9007199254740994', 'call.completed', { epoch: 'wrong' }),
    ], { latestSequence: '9007199254741099', hasMore: true }))
    expect(next.cursor).toBe('9007199254740993')
    expect(next.events).toHaveLength(1)
    expect(next.calls[0].status).toBe('waiting_for_user')
  })

  it('keeps only contract-provided structural output and file facts', () => {
    const output = { continuationID: 'cont', status: 'running', exitCode: null, commandOK: null, timedOut: false, stdoutTotalBytes: 500, stderrTotalBytes: 2, stdoutTruncated: true, stderrTruncated: false }
    const fileChange = { action: 'modify', paths: ['src/app.ts'], filesChanged: 1, insertions: 2, deletions: 3, statsKnown: true, truncated: false }
    const state = reduceExecutionPage(replaceExecution(snapshot()), page([event('2', 'output.summary', { output }), event('3', 'file.changed', { fileChange })]))
    expect(state.events.map(item => [item.output, item.fileChange])).toEqual([[output, null], [null, fileChange]])
    expect(state.calls[0].continuationID).toBe('cont')
  })

  it('validates UTF-8 byte length and rejects empty or malformed text', () => {
    expect(validInsertionText('x'.repeat(8192))).toBe(true)
    expect(validInsertionText('界'.repeat(2730))).toBe(true)
    expect(validInsertionText('界'.repeat(2731))).toBe(false)
    expect(validInsertionText('  ')).toBe(false)
    expect(validInsertionText('\u0085')).toBe(false)
    expect(validInsertionText('\ufeff')).toBe(true)
    expect(validInsertionText('\ud800')).toBe(false)
    expect(validInsertionText('😀')).toBe(true)
  })
})
