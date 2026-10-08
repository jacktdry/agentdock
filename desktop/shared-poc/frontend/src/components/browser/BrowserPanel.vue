<script setup lang="ts">
import { onMounted } from 'vue'
import { useBrowserStore } from '../../stores/browser'
import { useI18n } from '../../i18n'
import BrowserSnapshotDetails from './BrowserSnapshotDetails.vue'
const store = useBrowserStore()
const { t } = useI18n()
onMounted(() => { void store.refresh() })
</script>

<template>
  <section class="panel browser-panel" aria-labelledby="browser-title" :aria-busy="store.busy">
    <h2 id="browser-title">{{ t('browser.title') }}</h2>
    <p class="hint">{{ t('browser.scope') }}</p>
    <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
    <div role="status" aria-live="polite" aria-atomic="true">
      <p v-if="store.busy">{{ t('browser.loading') }}</p>
      <p v-else-if="store.snapshot" class="sr-only">{{ t(store.availabilityKey) }} · {{ t(store.stateKey) }}</p>
    </div>
    <p v-if="store.error" class="error" role="alert">{{ t(store.error) }}</p>
    <div v-if="store.snapshot">
      <dl class="status-grid">
        <div><dt>{{ t('browser.availability') }}</dt><dd>{{ t(store.availabilityKey) }}</dd></div>
        <div><dt>{{ t('browser.state') }}</dt><dd>{{ t(store.stateKey) }}</dd></div>
        <div><dt>{{ t('browser.browser_enabled') }}</dt><dd>{{ t(store.snapshot.browserEnabled ? 'common.yes' : 'common.no') }}</dd></div>
        <div><dt>{{ t('browser.acp_enabled') }}</dt><dd>{{ t(store.snapshot.acpEnabled ? 'common.yes' : 'common.no') }}</dd></div>
        <div><dt>{{ t('browser.observed_at') }}</dt><dd><time :datetime="store.snapshot.observedAt">{{ new Date(store.snapshot.observedAt).toISOString() }}</time></dd></div>
        <div><dt>{{ t('browser.company_required_edge_policies') }}</dt><dd>{{ store.snapshot.companyRequiredEdgePolicies }}</dd></div>
      </dl>
      <BrowserSnapshotDetails v-if="store.snapshot.availability === 'available'" :snapshot="store.snapshot" />
    </div>
    <p class="hint">{{ t('browser.routes') }}</p>
  </section>
</template>
