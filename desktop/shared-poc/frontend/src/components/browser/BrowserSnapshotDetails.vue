<script setup lang="ts">
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { useI18n } from '../../i18n'
defineProps<{ snapshot: Snapshot }>()
const { t } = useI18n()
</script>

<template>
  <div>
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
  </div>
</template>
