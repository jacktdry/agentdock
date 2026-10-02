import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref } from 'vue'
import {
  clientError,
  desktopApi,
  type ActivityBatch,
  type ActivityEnvelope,
  type ActivityProbeStatus,
  type ActivityStreamHandle,
  type ActivityStreamState,
  type APIError,
} from '../api/desktopApi'
import { appendBoundedHistory } from '../utils/boundedHistory'

const emptyStatus = (): ActivityProbeStatus => ({
  running: false,
  rateHz: 0,
  epoch: '',
  latestSequence: '0',
  publishedTotal: '0',
  deliveredTotal: '0',
  droppedTotal: '0',
  sourceDroppedTotal: '0',
  transportDroppedTotal: '0',
  queueDepth: 0,
  queueCapacity: 0,
})

export const useActivityStore = defineStore('activity', () => {
  const status = ref<ActivityProbeStatus>(emptyStatus())
  const history = ref<ActivityEnvelope[]>([])
  const lastBatchSize = ref(0)
  const batchIntervalMs = ref(100)
  const busy = ref(false)
  const error = ref<APIError | null>(null)
  const streamState = ref<ActivityStreamState>('closed')
  const summaryAnnouncement = ref('Synthetic activity stream is stopped.')
  const observedGap = ref('0')
  let streamHandle: ActivityStreamHandle | null = null
  let lastSequence = '0'
  let activeEpoch = ''

  const loadPercent = computed(() => {
    if (!status.value.queueCapacity) return 0
    return Math.round((status.value.queueDepth / status.value.queueCapacity) * 100)
  })

  function noteGap(batch: ActivityBatch) {
    if (activeEpoch && batch.epoch !== activeEpoch) {
      history.value = []
      lastSequence = '0'
      observedGap.value = '0'
    }
    activeEpoch = batch.epoch

    if (lastSequence !== '0') {
      const expected = BigInt(lastSequence) + 1n
      const actual = BigInt(batch.firstSequence)
      if (actual > expected) {
        observedGap.value = (BigInt(observedGap.value) + (actual - expected)).toString()
      }
    }

    let previous = BigInt(batch.firstSequence)
    for (const event of batch.events.slice(1)) {
      const current = BigInt(event.sequence)
      if (current > previous + 1n) {
        observedGap.value = (BigInt(observedGap.value) + (current - previous - 1n)).toString()
      }
      previous = current
    }
    lastSequence = batch.lastSequence
  }

  function acceptBatch(batch: ActivityBatch) {
    noteGap(batch)
    history.value = appendBoundedHistory(history.value, batch.events)
    lastBatchSize.value = batch.events.length
    status.value = {
      ...status.value,
      running: true,
      epoch: batch.epoch,
      latestSequence: batch.lastSequence,
      publishedTotal: batch.publishedTotal,
      deliveredTotal: batch.deliveredTotal,
      droppedTotal: (BigInt(batch.sourceDroppedTotal) + BigInt(batch.transportDroppedTotal)).toString(),
      sourceDroppedTotal: batch.sourceDroppedTotal,
      transportDroppedTotal: batch.transportDroppedTotal,
      queueDepth: batch.queueDepth,
      queueCapacity: batch.queueCapacity,
    }
    summaryAnnouncement.value =
      'Activity stream active. ' +
      batch.deliveredTotal +
      ' delivered, ' +
      status.value.droppedTotal +
      ' dropped.'
  }

  function updateStreamState(state: ActivityStreamState) {
    streamState.value = state
    if ((state === 'closed' || state === 'error') && status.value.running) {
      summaryAnnouncement.value =
        'Activity transport disconnected; new batches will be dropped until it reconnects.'
    }
  }

  function subscribe() {
    if (!streamHandle) {
      const handle = desktopApi.openActivityStream(
        acceptBatch,
        (state) => {
          updateStreamState(state)
          if ((state === 'closed' || state === 'error') && streamHandle === handle) {
            streamHandle = null
          }
        },
        (contractError) => {
          error.value = contractError
          summaryAnnouncement.value = 'Activity contract rejected: ' + contractError.message
        },
      )
      streamHandle = handle
    }
    return streamHandle.ready
  }

  function unsubscribeActivity() {
    streamHandle?.close()
    streamHandle = null
    streamState.value = 'closed'
  }

  onScopeDispose(unsubscribeActivity)

  async function refreshStatus() {
    try {
      status.value = await desktopApi.activityStatus()
    } catch (caught) {
      error.value = clientError('activity_status_call_failed', caught)
    }
  }

  async function start(rateHz: number, intervalMs: number) {
    busy.value = true
    error.value = null
    try {
      await subscribe()
      const result = await desktopApi.startActivityProbe(rateHz, intervalMs)
      error.value = result.error ?? null
      status.value = result.status ?? emptyStatus()
      batchIntervalMs.value = intervalMs
      summaryAnnouncement.value = error.value
        ? 'Activity probe failed: ' + error.value.message
        : 'Activity probe started at ' + rateHz + ' events per second.'
    } catch (caught) {
      unsubscribeActivity()
      error.value = clientError('activity_start_call_failed', caught)
      summaryAnnouncement.value = 'Activity probe failed: ' + error.value.message
    } finally {
      busy.value = false
    }
  }

  async function stop() {
    busy.value = true
    error.value = null
    try {
      status.value = await desktopApi.stopActivityProbe()
      summaryAnnouncement.value = 'Synthetic activity source stopped.'
    } catch (caught) {
      error.value = clientError('activity_stop_call_failed', caught)
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
    observedGap,
    loadPercent,
    subscribe,
    unsubscribeActivity,
    refreshStatus,
    start,
    stop,
    clearHistory,
  }
})
