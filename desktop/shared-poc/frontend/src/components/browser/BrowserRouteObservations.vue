<script setup lang="ts">
import { computed } from 'vue'
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { useI18n, type MessageKey } from '../../i18n'

const props = defineProps<{ snapshot: Snapshot }>()
const { t } = useI18n()

// Configuration counts cannot prove authorization, the exact profile,
// CDP reachability, real browser health, or a verified connector.
const companyRouteKey = computed<MessageKey>(() => {
  const s = props.snapshot
  if (s.availability === 'core_unavailable' ||
    ![s.companyRequiredEdgePolicies, s.configuredAuthenticatedEdgeProfiles, s.configuredConnectors]
      .every(value => Number.isSafeInteger(value) && value >= 0)) {
    return 'browser.route_company_unknown'
  }
  if (s.companyRequiredEdgePolicies === 0) return 'browser.route_company_no_policy'
  if (s.configuredAuthenticatedEdgeProfiles === 0 || s.configuredConnectors === 0) {
    return 'browser.route_company_incomplete'
  }
  return 'browser.route_company_unverified'
})
const managedRouteKey = computed<MessageKey>(() => {
  const s = props.snapshot
  const counts = [s.authenticatedTransportOwners, s.owners, s.expiredLeases, s.releasingLeases,
    s.failedLeases, s.unownedLeases, s.failedWorkers, s.managedOrphans, s.externalOrphans]
  const anomalies = [s.expiredLeases, s.releasingLeases, s.failedLeases, s.unownedLeases,
    s.failedWorkers, s.managedOrphans, s.externalOrphans]
  const budget = s.sharedPoolBudget
  const invalidBudget = s.managedPoolMode === 'experimental_shared' &&
    (!budget || ![budget.capacity, budget.occupied, budget.available, budget.claimed, budget.uncertain]
      .every(value => Number.isSafeInteger(value) && value >= 0) ||
      budget.capacity <= 0 || budget.occupied + budget.available !== budget.capacity || budget.uncertain > 0)
  if (s.availability !== 'available' || s.stale || s.lifecycleError ||
    !['idle', 'leases_present'].includes(s.state) ||
    !['exclusive', 'experimental_shared'].includes(s.managedPoolMode) ||
    counts.some(value => !Number.isSafeInteger(value) || value < 0) ||
    s.authenticatedTransportOwners > s.owners || anomalies.some(value => value > 0) || invalidBudget) {
    return 'browser.route_managed_unavailable'
  }
  return 'browser.route_managed_unverified'
})
</script>

<template>
  <section aria-labelledby="browser-route-observations-title">
    <h3 id="browser-route-observations-title">{{ t('browser.route_observations') }}</h3>
    <p class="hint">{{ t('browser.configuration_hint') }}</p>
    <dl class="status-grid">
      <div><dt>{{ t('browser.configured_connectors') }}</dt><dd>{{ snapshot.configuredConnectors }}</dd></div>
      <div><dt>{{ t('browser.configured_authenticated_edge_profiles') }}</dt><dd>{{ snapshot.configuredAuthenticatedEdgeProfiles }}</dd></div>
      <div><dt>{{ t('browser.configured_required_external_policies') }}</dt><dd>{{ snapshot.configuredRequiredExternalPolicies }}</dd></div>
      <div><dt>{{ t('browser.company_required_edge_policies') }}</dt><dd>{{ snapshot.companyRequiredEdgePolicies }}</dd></div>
      <div><dt>{{ t('browser.connector_health') }}</dt><dd>{{ t('browser.connector_health_not_observed') }}</dd></div>
    </dl>
    <p class="hint">{{ t('browser.connector_health_hint') }}</p>
    <section aria-labelledby="browser-route-guide-title">
      <h4 id="browser-route-guide-title">{{ t('browser.route_guide_title') }}</h4>
      <dl class="status-grid">
        <div><dt>{{ t('browser.route_managed_title') }}</dt><dd>{{ t(managedRouteKey) }}</dd></div>
        <div><dt>{{ t('browser.route_company_title') }}</dt><dd>{{ t(companyRouteKey) }}</dd></div>
        <div><dt>{{ t('browser.route_explicit_title') }}</dt><dd>{{ t('browser.route_explicit_unobserved') }}</dd></div>
      </dl>
    </section>
    <template v-if="snapshot.availability === 'available'">
      <h4>{{ t('browser.retained_routes') }}</h4>
      <p class="hint">{{ t('browser.retained_routes_hint') }}</p>
      <dl class="status-grid">
        <div><dt>{{ t('browser.managed_leases') }}</dt><dd>{{ snapshot.managedLeases }}</dd></div>
        <div><dt>{{ t('browser.required_external_leases') }}</dt><dd>{{ snapshot.requiredExternalLeases }}</dd></div>
        <div><dt>{{ t('browser.explicit_external_leases') }}</dt><dd>{{ snapshot.explicitExternalLeases }}</dd></div>
      </dl>
    </template>
  </section>
</template>
