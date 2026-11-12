<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useBrowserStore } from '../../stores/browser'
import { currentLocale, useI18n } from '../../i18n'
import BrowserRouteObservations from './BrowserRouteObservations.vue'
import BrowserSnapshotDetails from './BrowserSnapshotDetails.vue'
import BrowserReadinessGuidance from './BrowserReadinessGuidance.vue'
const store = useBrowserStore()
const { t } = useI18n()
const effectiveSnapshot = computed(() => store.snapshot ? {
  ...store.snapshot, stale: store.snapshot.stale || store.clientStale,
} : null)
const observedAtLabel = computed(() => {
  const raw = store.snapshot?.observedAt
  if (!raw) return t('common.unknown')
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return t('common.unknown')
  return new Intl.DateTimeFormat(currentLocale.value, {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(date)
})
onMounted(() => { void store.refresh() })
</script>

<template>
  <section class="panel browser-panel" aria-labelledby="browser-title" :aria-busy="store.busy">
    <h2 id="browser-title">{{ t('browser.title') }}</h2>
    <BrowserReadinessGuidance :snapshot="effectiveSnapshot" :error="store.error" />
    <p class="hint">{{ t('browser.scope') }}</p>
    <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
    <div class="operation-status" role="status" aria-live="polite" aria-atomic="true">
      <p v-if="store.busy">{{ t('browser.loading') }}</p>
      <p v-else-if="store.clientStale" class="hint">{{ t('browser.stale') }}</p>
      <p v-else-if="store.snapshot" class="sr-only">{{ t(store.availabilityKey) }} · {{ t(store.stateKey) }}</p>
    </div>
    <p v-if="store.error" class="error" role="alert">{{ t(store.error) }}</p>
    <div v-if="store.snapshot">
      <dl class="status-grid">
        <div><dt>{{ t('browser.availability') }}</dt><dd>{{ t(store.availabilityKey) }}</dd></div>
        <div><dt>{{ t('browser.state') }}</dt><dd>{{ t(store.stateKey) }}</dd></div>
        <div><dt>{{ t('browser.browser_enabled') }}</dt><dd>{{ t(store.snapshot.browserEnabled ? 'common.yes' : 'common.no') }}</dd></div>
        <div><dt>{{ t('browser.acp_enabled') }}</dt><dd>{{ t(store.snapshot.acpEnabled ? 'common.yes' : 'common.no') }}</dd></div>
        <div><dt>{{ t('browser.observed_at') }}</dt><dd><time :datetime="store.snapshot.observedAt">{{ observedAtLabel }}</time></dd></div>
      </dl>
      <BrowserRouteObservations :snapshot="effectiveSnapshot!" />
      <BrowserSnapshotDetails :snapshot="effectiveSnapshot!" />
    </div>
    <p class="hint">{{ t('browser.routes') }}</p>
  </section>
</template>
