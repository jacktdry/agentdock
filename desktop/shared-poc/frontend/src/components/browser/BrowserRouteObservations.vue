<script setup lang="ts">
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { useI18n } from '../../i18n'
defineProps<{ snapshot: Snapshot }>()
const { t } = useI18n()
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
