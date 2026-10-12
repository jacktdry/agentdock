<script setup lang="ts">
import { computed } from 'vue'
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import type { MessageKey } from '../../i18n'
import { useI18n } from '../../i18n'
const props = defineProps<{ snapshot: Snapshot }>()
const modeKey = computed<MessageKey>(() => {
  if (props.snapshot.availability !== 'available') return 'browser.pool_unknown'
  switch (props.snapshot.managedPoolMode) {
    case 'exclusive': return 'browser.pool_exclusive'
    case 'experimental_shared': return 'browser.pool_shared'
    default: return 'browser.pool_unknown'
  }
})
const { t } = useI18n()
</script>

<template>
  <div>
    <dl class="status-grid">
      <div><dt>{{ t('browser.pool_mode') }}</dt><dd>{{ t(modeKey) }}</dd></div>
    </dl>
    <p class="hint">{{ t('browser.pool_hint') }}</p>
    <dl v-if="snapshot.availability === 'available' && snapshot.managedPoolMode === 'experimental_shared' && snapshot.sharedPoolBudget" class="status-grid">
      <div><dt>{{ t('browser.pool_capacity') }}</dt><dd>{{ snapshot.sharedPoolBudget.capacity }}</dd></div>
      <div><dt>{{ t('browser.pool_occupied') }}</dt><dd>{{ snapshot.sharedPoolBudget.occupied }}</dd></div>
      <div><dt>{{ t('browser.pool_available') }}</dt><dd>{{ snapshot.sharedPoolBudget.available }}</dd></div>
      <div><dt>{{ t('browser.pool_claimed') }}</dt><dd>{{ snapshot.sharedPoolBudget.claimed }}</dd></div>
      <div><dt>{{ t('browser.pool_uncertain') }}</dt><dd>{{ snapshot.sharedPoolBudget.uncertain }}</dd></div>
    </dl>
    <template v-if="snapshot.availability === 'available'">
    <dl class="status-grid">
      <div><dt>{{ t('browser.stale_flag') }}</dt><dd>{{ t(snapshot.stale ? 'common.yes' : 'common.no') }}</dd></div>
    </dl>
    <p class="hint">{{ t('browser.observation_hint') }}</p>
    <p class="hint">{{ t('browser.policy_hint') }}</p>
    <h3>{{ t('browser.retained') }}</h3>
    <p class="hint">{{ t('browser.retained_hint') }}</p>
    <dl class="status-grid">
      <div><dt>{{ t('browser.owners') }}</dt><dd>{{ snapshot.owners }}</dd></div>
      <div><dt>{{ t('browser.leases') }}</dt><dd>{{ snapshot.leases }}</dd></div>
      <div><dt>{{ t('browser.workers') }}</dt><dd>{{ snapshot.workers }}</dd></div>
      <div><dt>{{ t('browser.active_leases') }}</dt><dd>{{ snapshot.activeLeases }}</dd></div>
      <div><dt>{{ t('browser.ready_workers') }}</dt><dd>{{ snapshot.readyWorkers }}</dd></div>
      <div><dt>{{ t('browser.failed_workers') }}</dt><dd>{{ snapshot.failedWorkers }}</dd></div>
    </dl>
    <h3>{{ t('browser.operations') }}</h3>
    <p class="hint">{{ t('browser.operations_hint') }}</p>
    <dl class="status-grid">
      <div><dt>{{ t('browser.active_operations') }}</dt><dd>{{ snapshot.activeOperations }}</dd></div>
      <div><dt>{{ t('browser.queued_operations') }}</dt><dd>{{ snapshot.queuedOperations }}</dd></div>
      <div><dt>{{ t('browser.max_concurrency') }}</dt><dd>{{ snapshot.maxConcurrency }}</dd></div>
      <div><dt>{{ t('browser.queue_capacity') }}</dt><dd>{{ snapshot.queueCapacity }}</dd></div>
    </dl>
    <h3>{{ t('browser.anomalies') }}</h3>
    <p class="hint">{{ t('browser.anomalies_hint') }}</p>
    <dl class="status-grid">
      <div><dt>{{ t('browser.expired_leases') }}</dt><dd>{{ snapshot.expiredLeases }}</dd></div>
      <div><dt>{{ t('browser.releasing_leases') }}</dt><dd>{{ snapshot.releasingLeases }}</dd></div>
      <div><dt>{{ t('browser.failed_leases') }}</dt><dd>{{ snapshot.failedLeases }}</dd></div>
      <div><dt>{{ t('browser.unowned_leases') }}</dt><dd>{{ snapshot.unownedLeases }}</dd></div>
      <div><dt>{{ t('browser.managed_orphans') }}</dt><dd>{{ snapshot.managedOrphans }}</dd></div>
      <div><dt>{{ t('browser.external_orphans') }}</dt><dd>{{ snapshot.externalOrphans }}</dd></div>
      <div><dt>{{ t('browser.lifecycle_error') }}</dt><dd>{{ t(snapshot.lifecycleError ? 'common.yes' : 'common.no') }}</dd></div>
    </dl>
    </template>
  </div>
</template>
