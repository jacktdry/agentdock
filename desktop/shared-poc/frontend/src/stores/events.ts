import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref } from 'vue'
import {
  desktopApi,
  type APIError,
  type EventBatch,
  type EventSourceStatus,
  type EventStreamHandle,
  type EventStreamState,
  type SyntheticEvent,
} from '../api/desktopApi'
import { appendBoundedHistory } from '../utils/boundedHistory'

const emptyStatus = (): EventSourceStatus => ({
  running: false,
  rateHz: 0,
  produced: 0,
  delivered: 0,
  dropped: 0,
  queueDropped: 0,
  transportDropped: 0,
  queueDepth: 0,
  queueCapacity: 0,
})

export const useEventsStore = defineStore('events', () => {
  const status = ref<EventSourceStatus>(emptyStatus())
  const history = ref<SyntheticEvent[]>([])
  const lastBatchSize = ref(0)
  const batchIntervalMs = ref(100)
  const busy = ref(false)
  const error = ref<APIError | null>(null)
  const streamState = ref<EventStreamState>('closed')
  const summaryAnnouncement = ref('Synthetic event stream is stopped.')
  let streamHandle: EventStreamHandle | null = null

  const loadPercent = computed(() => {
    if (!status.value.queueCapacity) return 0
    return Math.round((status.value.queueDepth / status.value.queueCapacity) * 100)
  })

  function acceptBatch(batch: EventBatch) {
    history.value = appendBoundedHistory(history.value, batch.events)
    lastBatchSize.value = batch.events.length
    batchIntervalMs.value = batch.batchIntervalMs
    status.value = {
      ...status.value,
      running: true,
      produced: batch.produced,
      delivered: batch.delivered,
      dropped: batch.dropped,
      queueDropped: batch.queueDropped,
      transportDropped: batch.transportDropped,
      queueDepth: batch.queueDepth,
      queueCapacity: batch.queueCapacity,
    }
    summaryAnnouncement.value =
      `Event stream active. ${batch.delivered} delivered, ${batch.dropped} dropped.`
  }

  function updateStreamState(state: EventStreamState) {
    streamState.value = state
    if (state === 'closed' || state === 'error') {
      if (status.value.running) {
        summaryAnnouncement.value = 'Event transport disconnected; new batches will be dropped until it reconnects.'
      }
    }
  }

  function subscribe() {
    if (!streamHandle) {
      const handle = desktopApi.openEventStream(acceptBatch, (state) => {
        updateStreamState(state)
        if ((state === 'closed' || state === 'error') && streamHandle === handle) {
          streamHandle = null
        }
      })
      streamHandle = handle
    }
    return streamHandle.ready
  }

  function unsubscribeEvents() {
    streamHandle?.close()
    streamHandle = null
    streamState.value = 'closed'
  }

  onScopeDispose(unsubscribeEvents)

  async function refreshStatus() {
    try {
      status.value = await desktopApi.eventStatus()
    } catch (caught) {
      error.value = {
        code: 'event_status_call_failed',
        message: caught instanceof Error ? caught.message : String(caught),
      }
    }
  }

  async function start(rateHz: number, intervalMs: number) {
    busy.value = true
    error.value = null
    try {
      await subscribe()
      const result = await desktopApi.startEvents(rateHz, intervalMs)
      error.value = result.error ?? null
      status.value = result.status ?? emptyStatus()
      batchIntervalMs.value = intervalMs
      summaryAnnouncement.value = error.value
        ? `Event stream failed: ${error.value.message}`
        : `Event stream started at ${rateHz} events per second.`
    } catch (caught) {
      unsubscribeEvents()
      error.value = {
        code: 'event_start_call_failed',
        message: caught instanceof Error ? caught.message : String(caught),
      }
      summaryAnnouncement.value = `Event stream failed: ${error.value.message}`
    } finally {
      busy.value = false
    }
  }

  async function stop() {
    busy.value = true
    error.value = null
    try {
      status.value = await desktopApi.stopEvents()
      summaryAnnouncement.value = 'Synthetic event stream stopped.'
    } catch (caught) {
      error.value = {
        code: 'event_stop_call_failed',
        message: caught instanceof Error ? caught.message : String(caught),
      }
    } finally {
      busy.value = false
    }
  }

  function clearHistory() {
    history.value = []
  }

  return {
    status,
    history,
    lastBatchSize,
    batchIntervalMs,
    busy,
    error,
    streamState,
    summaryAnnouncement,
    loadPercent,
    subscribe,
    unsubscribeEvents,
    refreshStatus,
    start,
    stop,
    clearHistory,
  }
})
