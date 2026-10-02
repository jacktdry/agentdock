<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import { useEventsStore } from '../../stores/events'
import { usePreferencesStore } from '../../stores/preferences'

const events = useEventsStore()
const preferencesStore = usePreferencesStore()
const { status, history, lastBatchSize, busy, error, streamState, summaryAnnouncement, loadPercent } =
  storeToRefs(events)
const rateHz = ref(500)

const intervalMs = computed(() => preferencesStore.preferences.batchIntervalMs || 100)

function start() {
  void events.start(rateHz.value, intervalMs.value)
}
</script>

<template>
  <section class="panel panel-wide" aria-labelledby="events-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">Backpressure probe</p>
        <h2 id="events-title">Synthetic event stream</h2>
      </div>
      <div class="stream-state-group">
        <span class="state-pill" :data-state="streamState === 'open' ? 'healthy' : 'stopped'">
          Transport: {{ streamState }}
        </span>
        <span class="state-pill" :data-state="status.running ? 'healthy' : 'stopped'">
          Source: {{ status.running ? 'running' : 'stopped' }}
        </span>
      </div>
    </div>

    <div class="stream-toolbar">
      <label for="event-rate">Producer rate</label>
      <input id="event-rate" v-model.number="rateHz" type="number" min="1" max="2000" step="100">
      <span>events/s</span>
      <button type="button" :disabled="busy || status.running" @click="start">Start stream</button>
      <button type="button" :disabled="busy || !status.running" @click="events.stop">Stop</button>
      <button type="button" :disabled="history.length === 0" @click="events.clearHistory">Clear log</button>
    </div>

    <dl class="metric-grid">
      <div><dt>Produced</dt><dd>{{ status.produced }}</dd></div>
      <div><dt>Delivered</dt><dd>{{ status.delivered }}</dd></div>
      <div><dt>Total dropped</dt><dd>{{ status.dropped }}</dd></div>
      <div><dt>Queue dropped</dt><dd>{{ status.queueDropped }}</dd></div>
      <div><dt>Transport dropped</dt><dd>{{ status.transportDropped }}</dd></div>
      <div><dt>Queue</dt><dd>{{ status.queueDepth }} / {{ status.queueCapacity }}</dd></div>
      <div><dt>Queue load</dt><dd>{{ loadPercent }}%</dd></div>
      <div><dt>Last batch</dt><dd>{{ lastBatchSize }}</dd></div>
    </dl>

    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>
    <p class="sr-only" aria-live="polite">{{ summaryAnnouncement }}</p>

    <div class="event-log-wrap">
      <div class="log-heading">
        <span>Bounded frontend history</span>
        <span>{{ history.length }} / 200</span>
      </div>
      <ol class="event-log" aria-label="Synthetic event log" aria-live="off">
        <li v-for="event in [...history].reverse().slice(0, 50)" :key="event.sequence">
          <span>#{{ event.sequence }}</span>
          <time>{{ event.time }}</time>
        </li>
        <li v-if="history.length === 0" class="empty">Start the stream to exercise event batching.</li>
      </ol>
    </div>
  </section>
</template>
