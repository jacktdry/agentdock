<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { nexusConnectionKey, nexusErrorKey, nexusOrigin, nexusPairingKey, useNexus } from '../../features/nexusLogic'
import { currentLocale, useI18n } from '../../i18n'
const { t } = useI18n()
const { snapshot, busy, notice, refresh, dispose } = useNexus()
const origin = computed(() => nexusOrigin(snapshot.value?.safeOrigin ?? ''))
const observedAtLabel = computed(() => {
  const raw = snapshot.value?.observedAt
  if (!raw) return t('nexus.unknown')
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return t('nexus.unknown')
  return new Intl.DateTimeFormat(currentLocale.value, {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(date)
})
onMounted(refresh)
onBeforeUnmount(dispose)
</script>
<template>
  <section class="panel nexus-panel" aria-labelledby="nexus-title">
    <div class="section-heading">
      <h2 id="nexus-title">{{ t('nexus.title') }} <span class="state-pill nexus-development">{{ t('nexus.in_development') }}</span></h2>
      <button type="button" :disabled="busy" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
    <p class="hint" role="status">{{ t('nexus.help') }}</p>
    <dl class="status-grid" aria-live="polite">
      <div><dt>{{ t('nexus.pairing_state') }}</dt><dd>{{ t(nexusPairingKey(snapshot?.pairingState)) }}</dd></div>
      <div><dt>{{ t('nexus.connection_state') }}</dt><dd>{{ t(nexusConnectionKey(snapshot?.connectionState)) }}</dd></div>
      <div><dt>{{ t('nexus.origin') }}</dt><dd>{{ origin || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('nexus.token') }}</dt><dd>{{ !snapshot ? t('nexus.unknown') : t(snapshot.deviceTokenStored ? 'nexus.saved' : 'nexus.not_saved') }}</dd></div>
    </dl>
    <p class="hint">{{ t('nexus.observed') }} <time :datetime="snapshot?.observedAt">{{ observedAtLabel }}</time></p>
    <p class="operation-status" role="status" aria-live="polite" aria-atomic="true">
      {{ busy ? t('common.loading') : notice ? t(notice) : snapshot?.error ? t(nexusErrorKey(snapshot.error.code)) : '' }}
    </p>
    <details><summary>{{ t('nexus.advanced') }}</summary><p>{{ t('nexus.node') }} <code>{{ snapshot?.nodeId || t('nexus.unknown') }}</code></p></details>
  </section>
</template>
<style scoped>
.nexus-panel { min-width: 0; overflow-wrap: anywhere; }
.nexus-development { display: inline-block; vertical-align: middle; font-weight: 400; white-space: nowrap; }
.nexus-panel .status-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
@media (max-width: 560px) { .nexus-panel .status-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
