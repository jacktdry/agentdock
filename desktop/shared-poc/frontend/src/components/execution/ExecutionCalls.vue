<script setup lang="ts">
import type { ExecutionCall } from '../../api/executionContract'
import { useI18n } from '../../i18n'

defineProps<{ rows: { call: ExecutionCall; depth: number }[]; title: string }>()
const { t } = useI18n()
</script>

<template>
  <section>
    <h3>{{ title }} <span class="hint">({{ rows.length }})</span></h3>
    <p v-if="!rows.length" class="hint">{{ t('execution.no_calls') }}</p>
    <ul v-else class="execution-list">
      <li v-for="{ call, depth } in rows" :key="call.callID" class="execution-call"
        :style="{ marginInlineStart: `${Math.min(depth, 4) * 1.1}rem` }">
        <div class="panel-heading">
          <strong>{{ call.tool || t('execution.unknown') }}</strong>
          <span class="state-pill" :data-state="call.status">{{ t(`execution.status_${call.status}`) }}</span>
        </div>
        <p class="execution-meta">{{ t('execution.source') }}: {{ call.source || t('execution.unknown') }} · {{ call.callID }}</p>
        <p v-if="call.parentCallID" class="execution-meta">{{ t('execution.child_of', { id: call.parentCallID }) }}</p>
        <p v-if="call.status === 'waiting_for_user'" class="execution-attention">{{ t('execution.waiting_hint') }}</p>
        <dl class="execution-facts">
          <div v-if="call.startedAt"><dt>{{ t('execution.started') }}</dt><dd><time :datetime="call.startedAt">{{ call.startedAt }}</time></dd></div>
          <div v-if="call.updatedAt"><dt>{{ t('execution.updated') }}</dt><dd><time :datetime="call.updatedAt">{{ call.updatedAt }}</time></dd></div>
          <div v-if="call.completedAt"><dt>{{ t('execution.finished') }}</dt><dd><time :datetime="call.completedAt">{{ call.completedAt }}</time></dd></div>
          <div v-if="call.errorCode || call.errorCategory"><dt>{{ t('execution.error') }}</dt><dd>{{ call.errorCode }} / {{ call.errorCategory }}</dd></div>
          <div v-if="call.continuationID"><dt>{{ t('execution.continuation') }}</dt><dd>{{ call.continuationID }}</dd></div>
        </dl>
      </li>
    </ul>
  </section>
</template>
