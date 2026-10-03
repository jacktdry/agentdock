<script setup lang="ts">
import { computed } from 'vue'
import type { ExecutionEvent } from '../../api/executionContract'
import { useI18n } from '../../i18n'
const props = defineProps<{ events: ExecutionEvent[] }>()
const { t } = useI18n()
const recent = computed(() => [...props.events].reverse())
</script>

<template>
  <section>
    <h3>{{ t('execution.recent_events') }} <span class="hint">({{ events.length }} / 200)</span></h3>
    <p class="hint">{{ t('execution.safe_facts') }}</p>
    <ol class="execution-list execution-events" aria-live="off" :aria-label="t('execution.recent_events')">
      <li v-for="event in recent" :key="`${event.epoch}:${event.sequence}`" class="execution-call">
        <p class="execution-meta">#{{ event.sequence }} · {{ event.kind }} · {{ event.callID }}</p>
        <time :datetime="event.occurredAt">{{ event.occurredAt }}</time>
        <p v-if="event.errorCode || event.errorCategory">{{ t('execution.error') }}: {{ event.errorCode }} / {{ event.errorCategory }}</p>
        <dl v-if="event.output" class="execution-facts">
          <div><dt>{{ t('execution.continuation') }}</dt><dd>{{ event.output.continuationID || t('execution.unknown') }} · {{ event.output.status || t('execution.unknown') }}</dd></div>
          <div><dt>{{ t('execution.output_bytes') }}</dt><dd>{{ event.output.stdoutTotalBytes }} / {{ event.output.stderrTotalBytes }}</dd></div>
          <div><dt>{{ t('execution.truncation') }}</dt><dd>{{ event.output.stdoutTruncated ? t('common.yes') : t('common.no') }} / {{ event.output.stderrTruncated ? t('common.yes') : t('common.no') }}</dd></div>
          <div><dt>{{ t('execution.exit_code') }}</dt><dd>{{ event.output.exitCode ?? t('execution.unknown') }}</dd></div>
          <div><dt>{{ t('execution.timed_out') }}</dt><dd>{{ event.output.timedOut ? t('common.yes') : t('common.no') }}</dd></div>
        </dl>
        <template v-if="event.fileChange">
          <p>{{ event.fileChange.action }} · {{ t('execution.files', { count: event.fileChange.filesChanged }) }}</p>
          <p>{{ t('execution.file_stats') }}: {{ event.fileChange.statsKnown ? `+${event.fileChange.insertions} / −${event.fileChange.deletions}` : t('execution.unknown') }}</p>
          <p>{{ t('execution.paths_truncated') }}: {{ event.fileChange.truncated ? t('common.yes') : t('common.no') }}</p>
          <ul><li v-for="(path, index) in event.fileChange.paths" :key="index">{{ path }}</li></ul>
        </template>
      </li>
    </ol>
  </section>
</template>
