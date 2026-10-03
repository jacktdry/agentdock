import { defineStore } from 'pinia'
import { computed, onScopeDispose, shallowRef } from 'vue'
import { desktopApi, clientError, type APIError, type ExecutionStreamHandle, type ExecutionStreamState } from '../api/desktopApi'
import { type ExecutionStreamEnvelope } from '../api/executionContract'
import {
  emptyExecution, executionRows, isActiveCall, normalizeInsertionText, reconcileInsertion, reduceExecutionPage, replaceExecution, validInsertionText,
} from '../features/executionReducer'

export const useExecutionStore = defineStore('execution', () => {
  const state = shallowRef(emptyExecution())
  const streamState = shallowRef<ExecutionStreamState>('closed')
  const synchronized = shallowRef(false)
  const reconnectTotal = shallowRef('0')
  const transportDroppedTotal = shallowRef('0')
  const error = shallowRef<APIError | null>(null)
  const controlError = shallowRef<APIError | null>(null)
  const busy = shallowRef(false)
  const lastInsertionID = shallowRef('')
  const lastInsertionStatus = computed(() => state.value.insertions.find(item => item.insertionID === lastInsertionID.value)?.status ?? '')
  const activeRows = computed(() => executionRows(state.value.calls, true))
  const historyRows = computed(() => executionRows(state.value.calls, false))
  const attentionCount = computed(() => state.value.calls.filter(call => call.status === 'waiting_for_user').length)
  const targets = computed(() => state.value.calls.filter(call => isActiveCall(call) && call.insertionSupported))
  const canControl = computed(() => streamState.value === 'open' && synchronized.value && !busy.value)
  let handle: ExecutionStreamHandle | null = null
  let generation = 0
  let snapshotRevision = 0

  function accept(message: ExecutionStreamEnvelope) {
    reconnectTotal.value = message.reconnectTotal
    transportDroppedTotal.value = message.transportDroppedTotal
    if (message.kind === 'snapshot' && message.snapshot) {
      state.value = replaceExecution(message.snapshot)
      snapshotRevision++
      synchronized.value = false
      error.value = null
    } else if (message.kind === 'activity' && message.page) {
      if (!state.value.epoch || message.page.epoch !== state.value.epoch || message.page.gap) {
        synchronized.value = false
        error.value = clientError('execution_resync_required', '')
        return
      }
      state.value = reduceExecutionPage(state.value, message.page)
    } else if (message.kind === 'status') {
      if (message.code === 'execution_reconnecting') {
        synchronized.value = false
        error.value = null
      } else if (message.code === 'execution_synchronized') {
        synchronized.value = state.value.epoch !== ''
        error.value = null
      }
    } else if (message.kind === 'error') {
      synchronized.value = false
      error.value = clientError(message.code || 'execution_stream_failed', '')
    }
  }

  function disconnect() {
    generation++
    handle?.close()
    handle = null
    synchronized.value = false
    streamState.value = 'closed'
  }

  async function connect() {
    disconnect()
    const current = generation
    error.value = null
    streamState.value = 'connecting'
    try {
      handle = desktopApi.openExecutionStream(
        message => { if (current === generation) accept(message) },
        status => {
          if (current !== generation) return
          streamState.value = status
          if (status !== 'open') synchronized.value = false
        },
        failure => {
          if (current !== generation) return
          error.value = failure
          synchronized.value = false
        },
      )
      await handle.ready
    } catch {
      if (current !== generation) return
      disconnect()
      streamState.value = 'error'
      error.value = clientError('execution_stream_failed', '')
    }
  }

  async function enqueue(targetCallID: string, text: string): Promise<boolean> {
    controlError.value = null
    lastInsertionID.value = ''
    if (!canControl.value || !targets.value.some(call => call.callID === targetCallID) || !validInsertionText(text)) {
      controlError.value = clientError('insertion_invalid', '')
      return false
    }
    busy.value = true
    const epoch = state.value.epoch
    const revision = snapshotRevision
    try {
      const result = await desktopApi.enqueueInsertion(targetCallID, normalizeInsertionText(text))
      if (epoch !== state.value.epoch || revision !== snapshotRevision) {
        controlError.value = clientError('execution_resync_required', '')
        return false
      }
      if (result.insertion) {
        state.value = reconcileInsertion(state.value, result.insertion)
        lastInsertionID.value = result.insertion.insertionID
      }
      controlError.value = result.error
      const accepted = !result.error && result.ack && result.insertion?.status === 'accepted'
      if (!accepted && !result.error) controlError.value = clientError('insertion_not_accepted', '')
      return accepted
    } catch {
      controlError.value = clientError('insertion_enqueue_failed', '')
      return false
    } finally { busy.value = false }
  }

  async function cancel(insertionID: string) {
    if (!canControl.value || !state.value.insertions.some(item => item.insertionID === insertionID && item.status === 'accepted')) return
    busy.value = true
    controlError.value = null
    const epoch = state.value.epoch
    const revision = snapshotRevision
    try {
      const result = await desktopApi.cancelInsertion(insertionID)
      if (epoch !== state.value.epoch || revision !== snapshotRevision) {
        controlError.value = clientError('execution_resync_required', '')
        return
      }
      if (result.insertion) {
        state.value = reconcileInsertion(state.value, result.insertion)
        lastInsertionID.value = result.insertion.insertionID
      }
      controlError.value = result.error
      if (!result.error && !result.insertion) controlError.value = clientError('insertion_cancel_failed', '')
    } catch { controlError.value = clientError('insertion_cancel_failed', '') }
    finally { busy.value = false }
  }

  onScopeDispose(disconnect)
  return {
    state, streamState, synchronized, reconnectTotal, transportDroppedTotal, error, controlError, busy,
    lastInsertionStatus, activeRows, historyRows, attentionCount, targets, canControl, connect, disconnect, enqueue, cancel,
  }
})
