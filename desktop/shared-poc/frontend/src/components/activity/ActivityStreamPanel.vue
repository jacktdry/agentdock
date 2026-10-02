<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { useActivityStore } from '../../stores/activity'
import { usePreferencesStore } from '../../stores/preferences'

const { t } = useI18n()
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

const transportLabel = computed(() => {
  switch (streamState.value) {
    case 'open':
      return t('common.connected')
    case 'error':
      return t('common.unavailable')
    case 'closed':
      return t('common.stopped')
    default:
      return t('common.loading')
  }
})

function start() {
  void activity.start(rateHz.value, intervalMs.value)
}
</script>

<template>
  <section class="panel panel-wide" aria-labelledby="activity-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ t('activity.eyebrow') }}</p>
        <h2 id="activity-title">{{ t('activity.title') }}</h2>
      </div>
      <div class="stream-state-group">
        <span class="state-pill" :data-state="streamState === 'open' ? 'healthy' : 'stopped'">
          {{ t('activity.transport', { state: transportLabel }) }}
        </span>
        <span class="state-pill" :data-state="status.running ? 'healthy' : 'stopped'">
          {{ t('activity.probe', { state: status.running ? t('common.running') : t('common.stopped') }) }}
        </span>
      </div>
    </div>

    <div class="stream-toolbar">
      <label for="activity-rate">{{ t('activity.synthetic_rate') }}</label>
      <input id="activity-rate" v-model.number="rateHz" type="number" min="1" max="2000" step="100">
      <span>{{ t('activity.rate_unit') }}</span>
      <button type="button" :disabled="busy || status.running" @click="start">{{ t('activity.start_probe') }}</button>
      <button type="button" :disabled="busy || !status.running" @click="activity.stop">{{ t('common.stop') }}</button>
      <button type="button" :disabled="history.length === 0" @click="activity.clearHistory">{{ t('activity.clear_log') }}</button>
    </div>

    <dl class="metric-grid">
      <div><dt>{{ t('activity.published') }}</dt><dd>{{ status.publishedTotal }}</dd></div>
      <div><dt>{{ t('activity.delivered') }}</dt><dd>{{ status.deliveredTotal }}</dd></div>
      <div><dt>{{ t('activity.total_dropped') }}</dt><dd>{{ status.droppedTotal }}</dd></div>
      <div><dt>{{ t('activity.observed_gap') }}</dt><dd>{{ observedGap }}</dd></div>
      <div><dt>{{ t('activity.source_dropped') }}</dt><dd>{{ status.sourceDroppedTotal }}</dd></div>
      <div><dt>{{ t('activity.transport_dropped') }}</dt><dd>{{ status.transportDroppedTotal }}</dd></div>
      <div><dt>{{ t('activity.queue') }}</dt><dd>{{ status.queueDepth }} / {{ status.queueCapacity }}</dd></div>
      <div><dt>{{ t('activity.queue_load') }}</dt><dd>{{ loadPercent }}%</dd></div>
      <div><dt>{{ t('activity.last_batch') }}</dt><dd>{{ lastBatchSize }}</dd></div>
      <div><dt>{{ t('activity.latest_cursor') }}</dt><dd class="cursor-value">{{ status.latestSequence }}</dd></div>
    </dl>

    <p class="path" :title="status.epoch">{{ t('activity.epoch', { epoch: status.epoch || t('common.not_started') }) }}</p>
    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>
    <p class="sr-only" aria-live="polite">{{ summaryAnnouncement }}</p>

    <div class="event-log-wrap">
      <div class="log-heading">
        <span>{{ t('activity.history') }}</span>
        <span>{{ history.length }} / 200</span>
      </div>
      <ol class="event-log" :aria-label="t('activity.log_label')" aria-live="off">
        <li v-for="event in [...history].reverse().slice(0, 50)" :key="event.epoch + ':' + event.sequence">
          <span>#{{ event.sequence }}</span>
          <time>{{ event.occurredAt }}</time>
        </li>
        <li v-if="history.length === 0" class="empty">{{ t('activity.empty') }}</li>
      </ol>
    </div>
  </section>
</template>
