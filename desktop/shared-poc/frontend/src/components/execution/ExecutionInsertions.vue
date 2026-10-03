<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import type { ExecutionCall } from '../../api/executionContract'
import { insertionTextBytes, MAX_INSERTION_BYTES, validInsertionText, type VisibleInsertion } from '../../features/executionReducer'
import { useI18n } from '../../i18n'

const props = defineProps<{
  targets: ExecutionCall[]; insertions: VisibleInsertion[]; enabled: boolean; busy: boolean;
  feedback: string; submit: (target: string, text: string) => Promise<boolean>;
}>()
const emit = defineEmits<{ cancel: [id: string] }>()
const { t } = useI18n()
const target = shallowRef('')
const text = shallowRef('')
const bytes = computed(() => insertionTextBytes(text.value))
const valid = computed(() => props.enabled && validInsertionText(text.value) && props.targets.some(call => call.callID === target.value))
async function send() {
  if (!valid.value) return
  const submitted = text.value
  if (await props.submit(target.value, submitted) && text.value === submitted) text.value = ''
}
</script>

<template>
  <section aria-labelledby="execution-insertion-title">
    <h3 id="execution-insertion-title">{{ t('execution.insertions') }}</h3>
    <p id="execution-ack-hint" class="hint">{{ t('execution.ack_hint') }}</p>
    <form class="execution-form" @submit.prevent="send">
      <label for="execution-target">{{ t('execution.target') }}</label>
      <select id="execution-target" v-model="target" :disabled="!enabled || busy">
        <option value="">{{ t('execution.select_target') }}</option>
        <option v-for="call in targets" :key="call.callID" :value="call.callID">
          {{ call.tool }} · {{ call.callID }} · {{ t(`execution.status_${call.status}`) }}
        </option>
      </select>
      <label for="execution-text">{{ t('execution.insertion_text') }}</label>
      <textarea id="execution-text" v-model="text" rows="3" :maxlength="MAX_INSERTION_BYTES" :disabled="busy" aria-describedby="execution-byte-hint execution-ack-hint" />
      <p id="execution-byte-hint" class="hint">{{ t('execution.byte_limit', { bytes, limit: MAX_INSERTION_BYTES }) }}</p>
      <button type="submit" :disabled="!valid || busy">{{ t('execution.send') }}</button>
    </form>
    <p aria-live="polite" role="status">{{ feedback }}</p>
    <ul class="execution-list" :aria-label="t('execution.insertions')">
      <li v-for="item in insertions" :key="item.insertionID" class="execution-call">
        <div class="panel-heading">
          <strong>{{ item.insertionID }}</strong>
          <span class="state-pill">{{ t(`execution.insertion_${item.status}`) }}</span>
        </div>
        <p class="execution-meta">{{ t('execution.target') }}: {{ item.targetCallID }}</p>
        <p v-if="item.textBytes !== null" class="execution-meta">{{ t('execution.bytes', { bytes: item.textBytes }) }}</p>
        <p class="execution-meta">{{ t('execution.reason') }}: {{ item.reason || t('execution.reason_unknown') }}</p>
        <button v-if="item.status === 'accepted'" type="button" :disabled="!enabled || busy" @click="emit('cancel', item.insertionID)">
          {{ t('common.cancel') }} <span class="sr-only">{{ item.insertionID }}</span>
        </button>
      </li>
    </ul>
  </section>
</template>
