<script setup lang="ts">
import { computed } from 'vue'
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { useI18n, type MessageKey } from '../../i18n'
const props = defineProps<{ snapshot: Snapshot | null; error?: string | null }>()
const { t } = useI18n()
const guidanceKey = computed<MessageKey>(() => {
  const s = props.snapshot
  if (props.error || !s || s.availability === 'core_unavailable') return 'browser.next_core'
  if (s.availability !== 'available') {
    if (s.availability === 'browser_disabled') return 'browser.next_browser'
    if (s.availability === 'acp_disabled') return 'browser.next_acp'
    return 'browser.next_broker'
  }
  if (s.stale || s.state === 'stale' || s.lifecycleError ||
    Object.values(s).some(value => typeof value === 'number' && (!Number.isInteger(value) || value < 0)) ||
    s.authenticatedTransportOwners > s.owners ||
    [s.expiredLeases, s.releasingLeases, s.failedLeases, s.unownedLeases, s.failedWorkers,
      s.managedOrphans, s.externalOrphans].some(count => count !== 0) ||
    !['exclusive', 'experimental_shared'].includes(s.managedPoolMode) ||
    (s.managedPoolMode === 'experimental_shared' && (!s.sharedPoolBudget || s.sharedPoolBudget.uncertain !== 0 ||
      Object.values(s.sharedPoolBudget).some(value => !Number.isInteger(value) || value < 0)))) {
    return 'browser.next_cleanup'
  }
  if (!['idle', 'leases_present'].includes(s.state)) return 'browser.next_broker'
  // This snapshot has no verified connector health signal, regardless of the health string.
  if (s.companyRequiredEdgePolicies > 0) return 'browser.next_company'
  return 'browser.next_session'
})
const modeKey = computed<MessageKey | null>(() => {
  if (guidanceKey.value !== 'browser.next_session') return null
  return props.snapshot?.managedPoolMode === 'exclusive' ? 'browser.next_exclusive' : 'browser.next_shared'
})
</script>

<template>
  <section aria-labelledby="browser-next-title">
    <h3 id="browser-next-title">{{ t('browser.next_title') }}</h3>
    <p class="hint" aria-live="polite" aria-atomic="true">{{ t(guidanceKey) }}<template v-if="modeKey"> {{ t(modeKey) }}</template></p>
  </section>
</template>
