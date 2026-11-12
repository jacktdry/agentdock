<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ACPBrokerResources, ACPLifecycleUpdate, ACPSessionLifecycle } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { idleMinutes, policyKeys, lifecyclePolicies, lifecycleUpdate, safeMetadata, type LifecyclePolicy } from '../../features/acpLogic'
import { useI18n } from '../../i18n'
const props = defineProps<{ profileId: string; session: ACPSessionLifecycle; resources?: ACPBrokerResources; canClose: boolean; canUpdate: boolean }>()
const emit = defineEmits<{ close: [sessionId: string]; update: [update: ACPLifecycleUpdate] }>()
const { t, currentLocale } = useI18n()
const policy = ref(props.session.lifecyclePolicy)
const minutes = ref(props.session.idleCloseAfterMs ? props.session.idleCloseAfterMs / 60000 : idleMinutes.default)
watch(() => props.session, (session) => {
  policy.value = session.lifecyclePolicy
  minutes.value = session.idleCloseAfterMs ? session.idleCloseAfterMs / 60000 : idleMinutes.default
})
const update = computed(() => lifecycleUpdate(props.profileId, props.session.sessionId, policy.value, minutes.value))
const lastActive = computed(() => {
  const date = new Date(props.session.lastActiveAt)
  return Number.isNaN(date.getTime()) ? t('common.unavailable') : date.toLocaleString(currentLocale.value)
})
const knownPolicy = computed(() => lifecyclePolicies.includes(props.session.lifecyclePolicy as LifecyclePolicy))
const knownStatuses = ['ready', 'running', 'closed', 'failed', 'starting', 'cancelled'] as const
const statusLabel = computed(() => {
  const status = knownStatuses.find(status => status === props.session.status)
  return status ? t(`acp.status_${status}`) : t('common.unavailable')
})
</script>
<template>
  <li class="acp-session">
    <h4 class="safe-url">{{ t('acp.session') }}: {{ safeMetadata(session.sessionId) || t('common.unavailable') }}</h4>
    <dl class="status-grid">
      <div><dt>{{ t('acp.status') }}</dt><dd>{{ statusLabel }}</dd></div>
      <div><dt>{{ t('acp.policy') }}</dt><dd>{{ knownPolicy ? t(policyKeys[session.lifecyclePolicy as LifecyclePolicy]) : t('common.unavailable') }}<span v-if="session.lifecyclePolicy === 'idle-managed'"> · {{ t('acp.minutes', { count: session.idleCloseAfterMs / 60000 }) }}</span></dd></div>
      <div><dt>{{ t('acp.loaded') }}</dt><dd>{{ t(session.loaded ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.running') }}</dt><dd>{{ t(session.activeRunId ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.idle') }}</dt><dd>{{ t(session.idleManagedIdle ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.eligible') }}</dt><dd>{{ t(session.idleManagedEligible ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.last_active') }}</dt><dd>{{ lastActive }}</dd></div>
      <div><dt>{{ t('acp.failures') }}</dt><dd :class="{ error: session.autoCloseError }">{{ t(session.autoCloseError ? 'acp.auto_close_failed' : 'common.no') }}</dd></div>
    </dl>
    <dl v-if="resources" class="status-grid">
      <div><dt>{{ t('acp.browser_leases') }}</dt><dd>{{ resources.browser.activeLeases }}</dd></div>
      <div><dt>{{ t('acp.cleanup_issues') }}</dt><dd>{{ resources.browser.cleanupIssues }}</dd></div>
      <div><dt>{{ t('acp.observe') }}</dt><dd>{{ resources.computer.observeSessions }}</dd></div>
      <div><dt>{{ t('acp.act') }}</dt><dd>{{ resources.computer.actSessions }}</dd></div>
    </dl>
    <div class="actions">
      <label>{{ t('acp.policy') }}
        <select v-model="policy" :disabled="!canUpdate || !!session.closedAt">
          <option v-for="item in lifecyclePolicies" :key="item" :value="item">{{ t(policyKeys[item]) }}</option>
        </select>
      </label>
      <label v-if="policy === 'idle-managed'">{{ t('acp.ttl') }}
        <input v-model.number="minutes" type="number" :min="idleMinutes.min" :max="idleMinutes.max" step="any"
          :disabled="!canUpdate || !!session.closedAt" :aria-invalid="!update" />
      </label>
      <button type="button" :disabled="!canUpdate || !update || !!session.closedAt" @click="update && emit('update', update)">{{ t('acp.apply_policy') }}</button>
      <button type="button" :disabled="!canClose || !!session.closedAt" @click="emit('close', session.sessionId)">{{ t('acp.close') }}</button>
    </div>
    <p v-if="policy === 'idle-managed'" class="hint">{{ t('acp.ttl_hint') }}</p>
    <p v-if="!update" class="error" role="status">{{ t('acp.invalid_policy') }}</p>
  </li>
</template>
