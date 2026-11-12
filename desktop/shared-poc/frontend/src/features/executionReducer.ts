import {
  MAX_EXECUTION_EVENTS, MAX_VISIBLE_CALLS,
  type ExecutionCall, type ExecutionEvent, type ExecutionInsertion,
  type ExecutionPage, type ExecutionSnapshot, type ExecutionStatus, type InsertionStatus,
} from '../api/executionContract'

export const MAX_VISIBLE_INSERTIONS = 128
export const MAX_INSERTION_BYTES = 8192
// Stream placeholders have no preview, byte count or timestamps to invent.
export type VisibleInsertion = Omit<ExecutionInsertion, 'textPreview' | 'textBytes'> & { textBytes: number | null }
export interface ExecutionState {
  epoch: string
  cursor: string
  calls: ExecutionCall[]
  insertions: VisibleInsertion[]
  events: ExecutionEvent[]
}

export const emptyExecution = (): ExecutionState => ({ epoch: '', cursor: '0', calls: [], insertions: [], events: [] })
export const isActiveCall = (call: ExecutionCall) => call.status === 'running' || call.status === 'waiting_for_user'
const compareSequence = (a: string, b: string) => BigInt(a || '0') < BigInt(b || '0') ? -1 : BigInt(a || '0') > BigInt(b || '0') ? 1 : 0

function boundCalls(calls: ExecutionCall[]) {
  return [...calls].sort((a, b) => Number(isActiveCall(b)) - Number(isActiveCall(a)) ||
    compareSequence(b.endedSequence || b.startedSequence, a.endedSequence || a.startedSequence)).slice(0, MAX_VISIBLE_CALLS)
}

function boundInsertions(items: VisibleInsertion[]) {
  // Keep pending controls before terminal history, without an auxiliary ID history.
  return [...items].reverse().sort((a, b) => Number(b.status === 'accepted') - Number(a.status === 'accepted'))
    .slice(0, MAX_VISIBLE_INSERTIONS).reverse()
}

export function replaceExecution(snapshot: ExecutionSnapshot): ExecutionState {
  return {
    epoch: snapshot.epoch, cursor: snapshot.latestSequence,
    calls: boundCalls(snapshot.calls),
    insertions: boundInsertions(snapshot.insertions.map(({ textPreview: _preview, ...item }) => item)),
    events: [],
  }
}

const callStatuses: Record<string, ExecutionStatus> = {
  'call.started': 'running', 'call.waiting': 'waiting_for_user', 'call.resumed': 'running',
  'call.completed': 'completed', 'call.failed': 'failed', 'call.cancelled': 'cancelled',
}
const insertionStatuses: Record<string, InsertionStatus> = {
  'insertion.accepted': 'accepted', 'insertion.delivered': 'delivered', 'insertion.rejected': 'rejected',
  'insertion.expired': 'expired', 'insertion.cancelled': 'cancelled',
}

export function reduceExecutionPage(state: ExecutionState, page: ExecutionPage): ExecutionState {
  if (!state.epoch || page.epoch !== state.epoch || page.gap) return state
  let calls = [...state.calls]
  let insertions = [...state.insertions]
  let cursor = state.cursor
  const events = [...state.events]
  for (const event of page.events) {
    if (event.epoch !== state.epoch || compareSequence(event.sequence, cursor) <= 0) continue
    cursor = event.sequence
    events.push(event)
    if (events.length > MAX_EXECUTION_EVENTS) events.shift()
    const status = callStatuses[event.kind]
    if (status && event.callID) {
      const previous = calls.find(call => call.callID === event.callID)
      const terminal = status !== 'running' && status !== 'waiting_for_user'
      const call: ExecutionCall = {
        callID: event.callID, parentCallID: event.parentCallID || previous?.parentCallID || '',
        tool: event.tool || previous?.tool || '', source: event.source || previous?.source || '', status,
        insertionSupported: previous?.insertionSupported ?? event.insertionSupported,
        startedSequence: previous?.startedSequence || (event.kind === 'call.started' ? event.sequence : ''),
        startedAt: previous?.startedAt || (event.kind === 'call.started' ? event.occurredAt : ''),
        updatedAt: event.occurredAt, completedAt: terminal ? event.occurredAt : '',
        endedSequence: terminal ? event.sequence : '', errorCode: event.errorCode,
        errorCategory: event.errorCategory, continuationID: previous?.continuationID || '',
      }
      calls = boundCalls([...calls.filter(item => item.callID !== call.callID), call])
    }
    if (event.output?.continuationID) {
      calls = calls.map(call => call.callID === event.callID ? { ...call, continuationID: event.output!.continuationID } : call)
    }
    const insertionStatus = insertionStatuses[event.kind]
    if (insertionStatus && event.insertionID) {
      const previous = insertions.find(item => item.insertionID === event.insertionID)
      if (previous || insertionStatus === 'accepted') {
        const item: VisibleInsertion = previous ?? {
          insertionID: event.insertionID, targetCallID: event.callID, status: 'accepted', textBytes: null,
          createdAt: '', updatedAt: '', expiresAt: '', deliveredAt: '', reason: '',
        }
        // A delayed accepted event must not regress a terminal control response.
        if (!previous || previous.status === 'accepted' || insertionStatus !== 'accepted') {
          insertions = boundInsertions([...insertions.filter(value => value.insertionID !== item.insertionID), {
            ...item, status: insertionStatus,
          }])
        }
      }
    }
  }
  return { ...state, cursor, calls, insertions, events }
}

export function reconcileInsertion(state: ExecutionState, insertion: ExecutionInsertion): ExecutionState {
  const { textPreview: _preview, ...safe } = insertion
  const previous = state.insertions.find(item => item.insertionID === safe.insertionID)
  const item = previous && previous.status !== 'accepted' && safe.status === 'accepted'
    ? { ...safe, status: previous.status, reason: previous.reason } : safe
  return { ...state, insertions: boundInsertions([...state.insertions.filter(value => value.insertionID !== item.insertionID), item]) }
}

// Match Go strings.TrimSpace (Unicode White_Space); JS trim differs for NEL and BOM.
export function normalizeInsertionText(text: string) { return text.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '') }
export function insertionTextBytes(text: string) { return new TextEncoder().encode(normalizeInsertionText(text)).length }
export function validInsertionText(text: string) {
  // UTF-8 encoding replaces lone surrogates; reject them instead of silently changing user intent.
  return insertionTextBytes(text) > 0 && insertionTextBytes(text) <= MAX_INSERTION_BYTES &&
    !/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(text)
}

export function executionRows(calls: ExecutionCall[], active: boolean) {
  const group = calls.filter(call => isActiveCall(call) === active)
  const ids = new Set(group.map(call => call.callID))
  const visited = new Set<string>()
  const rows: { call: ExecutionCall; depth: number }[] = []
  function visit(call: ExecutionCall, depth: number) {
    if (visited.has(call.callID)) return
    visited.add(call.callID)
    rows.push({ call, depth })
    group.filter(child => child.parentCallID === call.callID).forEach(child => visit(child, depth + 1))
  }
  group.filter(call => !ids.has(call.parentCallID)).forEach(call => visit(call, 0))
  group.forEach(call => visit(call, 0))
  return rows
}
