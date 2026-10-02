<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import { useActivityStore } from '../../stores/activity'
import { usePreferencesStore } from '../../stores/preferences'

const activity = useActivityStore()
const preferencesStore = usePreferencesStore()
const {
  status,
  history,
  lastBatchSize,
  busy,
  error,
  streamState,
  summaryAnnouncement,
  observedGap,
  loadPercent,
} = storeToRefs(activity)
const rateHz = ref(500)

const intervalMs = computed(() => preferencesStore.preferences.batchIntervalMs || 100)

function start() {
  void activity.start(rateHz.value, intervalMs.value)
}
</script>

<template>
  <section class="panel panel-wide" aria-labelledby="activity-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">Bounded data-plane contract</p>
        <h2 id="activity-title">Activity stream</h2>
      </div>
      <div class="stream-state-group">
        <span class="state-pill" :data-state="streamState === 'open' ? 'healthy' : 'stopped'">
          Transport: {{ streamState }}
        </span>
        <span class="state-pill" :data-state="status.running ? 'healthy' : 'stopped'">
          Probe: {{ status.running ? 'running' : 'stopped' }}
        </span>
      </div>
    </div>

    <div class="stream-toolbar">
      <label for="activity-rate">Synthetic rate</label>
      <input id="activity-rate" v-model.number="rateHz" type="number" min="1" max="2000" step="100">
      <span>activities/s</span>
      <button type="button" :disabled="busy || status.running" @click="start">Start probe</button>
      <button type="button" :disabled="busy || !status.running" @click="activity.stop">Stop</button>
      <button type="button" :disabled="history.length === 0" @click="activity.clearHistory">Clear log</button>
    </div>

    <dl class="metric-grid">
      <div><dt>Published</dt><dd>{{ status.publishedTotal }}</dd></div>
      <div><dt>Delivered</dt><dd>{{ status.deliveredTotal }}</dd></div>
      <div><dt>Total dropped</dt><dd>{{ status.droppedTotal }}</dd></div>
      <div><dt>Observed gap</dt><dd>{{ observedGap }}</dd></div>
      <div><dt>Source dropped</dt><dd>{{ status.sourceDroppedTotal }}</dd></div>
      <div><dt>Transport dropped</dt><dd>{{ status.transportDroppedTotal }}</dd></div>
      <div><dt>Queue</dt><dd>{{ status.queueDepth }} / {{ status.queueCapacity }}</dd></div>
      <div><dt>Queue load</dt><dd>{{ loadPercent }}%</dd></div>
      <div><dt>Last batch</dt><dd>{{ lastBatchSize }}</dd></div>
      <div><dt>Latest cursor</dt><dd class="cursor-value">{{ status.latestSequence }}</dd></div>
    </dl>

    <p class="path" :title="status.epoch">Epoch: {{ status.epoch || 'not started' }}</p>
    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>
    <p class="sr-only" aria-live="polite">{{ summaryAnnouncement }}</p>

    <div class="event-log-wrap">
      <div class="log-heading">
        <span>Bounded frontend activity history</span>
        <span>{{ history.length }} / 200</span>
      </div>
      <ol class="event-log" aria-label="Synthetic activity log" aria-live="off">
        <li v-for="event in [...history].reverse().slice(0, 50)" :key="event.epoch + ':' + event.sequence">
          <span>#{{ event.sequence }}</span>
          <time>{{ event.occurredAt }}</time>
        </li>
        <li v-if="history.length === 0" class="empty">
          Start the probe to exercise the versioned Activity contract.
        </li>
      </ol>
    </div>
  </section>
</template>
