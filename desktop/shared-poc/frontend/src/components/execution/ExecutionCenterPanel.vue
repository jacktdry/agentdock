<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useExecutionStore } from '../../stores/execution'
import { errorMessage } from '../../api/errorMessage'
import { useI18n } from '../../i18n'
import ExecutionCalls from './ExecutionCalls.vue'
import ExecutionInsertions from './ExecutionInsertions.vue'
import ExecutionFacts from './ExecutionFacts.vue'

const execution = useExecutionStore()
const { t } = useI18n()
const feedback = computed(() => execution.controlError ? errorMessage(execution.controlError) :
  execution.lastInsertionStatus ? t(`execution.insertion_${execution.lastInsertionStatus}`) : '')
onMounted(() => { void execution.connect() })
onUnmounted(execution.disconnect)
</script>

<template>
  <section class="panel feature-stack" aria-labelledby="execution-title">
    <div class="panel-heading">
      <h2 id="execution-title">{{ t('execution.title') }}</h2>
      <button type="button" :disabled="execution.streamState === 'connecting'" @click="execution.connect">{{ t('execution.reconnect') }}</button>
    </div>
    <p aria-live="polite" role="status">
      {{ t(`execution.stream_${execution.streamState}`) }} · {{ execution.synchronized ? t('execution.synchronized') : t('execution.last_known') }}
    </p>
    <p v-if="execution.error" class="error" role="status">{{ errorMessage(execution.error) }}</p>
    <dl class="metric-grid">
      <div><dt>{{ t('execution.attention') }}</dt><dd>{{ execution.attentionCount }}</dd></div>
      <div><dt>{{ t('execution.reconnect_total') }}</dt><dd>{{ execution.reconnectTotal }}</dd></div>
      <div><dt>{{ t('execution.transport_dropped') }}</dt><dd>{{ execution.transportDroppedTotal }}</dd></div>
      <div><dt>{{ t('execution.cursor') }}</dt><dd class="execution-meta">{{ execution.state.cursor }}</dd></div>
    </dl>
    <p class="execution-meta">{{ t('execution.epoch') }}: {{ execution.state.epoch || t('execution.unknown') }}</p>
    <p class="hint">{{ t('execution.bounds') }}</p>
    <ExecutionCalls :rows="execution.activeRows" :title="t('execution.active_calls')" />
    <ExecutionInsertions :targets="execution.targets" :insertions="execution.state.insertions" :enabled="execution.canControl"
      :busy="execution.busy" :feedback="feedback" :submit="execution.enqueue" @cancel="execution.cancel" />
    <ExecutionCalls :rows="execution.historyRows" :title="t('execution.history')" />
    <ExecutionFacts :events="execution.state.events" />
  </section>
</template>
