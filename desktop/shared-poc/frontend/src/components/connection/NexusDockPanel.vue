<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'
import { desktopApi } from '../../api/desktopApi'
import { nexusConnectionKey, nexusErrorKey, nexusOrigin, nexusPairingKey, useNexus } from '../../features/nexusLogic'
import { useI18n } from '../../i18n'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import NexusPairForm from './NexusPairForm.vue'
const { t } = useI18n()
const { snapshot, busy, notice, result, canPair, canReconcile, refresh, finishMutation, dispose } = useNexus()
const recoveryGeneration = shallowRef('')
const origin = computed(() => nexusOrigin(snapshot.value?.safeOrigin ?? ''))
function confirmRecovery() {
  const generation = recoveryGeneration.value
  recoveryGeneration.value = ''
  if (canReconcile.value && generation === snapshot.value?.generation) {
    void finishMutation(desktopApi.nexusReconcile(generation))
  }
}
onMounted(refresh)
onBeforeUnmount(dispose)
</script>
<template>
  <section class="panel nexus-panel" aria-labelledby="nexus-title">
    <div class="section-heading">
      <h2 id="nexus-title">{{ t('nexus.title') }}</h2>
      <button type="button" :disabled="busy" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
    <p class="hint">{{ t('nexus.help') }}</p>
    <dl class="status-grid" aria-live="polite">
      <div><dt>{{ t('nexus.pairing_state') }}</dt><dd>{{ t(nexusPairingKey(snapshot?.pairingState)) }}</dd></div>
      <div><dt>{{ t('nexus.connection_state') }}</dt><dd>{{ t(nexusConnectionKey(snapshot?.connectionState)) }}</dd></div>
      <div><dt>{{ t('nexus.origin') }}</dt><dd>{{ origin || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('nexus.token') }}</dt><dd>{{ t(snapshot?.deviceTokenStored ? 'nexus.saved' : 'nexus.not_saved') }}</dd></div>
    </dl>
    <p class="hint">{{ t('nexus.observed') }} <time :datetime="snapshot?.observedAt">{{ snapshot?.observedAt || t('nexus.unknown') }}</time></p>
    <div role="status" aria-live="polite" aria-atomic="true">
      <p v-if="busy">{{ t('common.loading') }}</p>
      <p v-if="notice">{{ t(notice) }}</p>
      <p v-else-if="snapshot?.error">{{ t(nexusErrorKey(snapshot.error.code)) }}</p>
      <p v-if="result?.identitySaved">{{ t('nexus.identity_saved') }}</p>
      <p v-if="snapshot?.restartRequired || result?.restartRequired">{{ t('nexus.restart_required') }}</p>
    </div>
    <details><summary>{{ t('nexus.advanced') }}</summary><p>{{ t('nexus.node') }} <code>{{ snapshot?.nodeId || t('nexus.unknown') }}</code></p></details>
    <NexusPairForm :snapshot="snapshot" :enabled="canPair" :busy="busy" @submitted="finishMutation" />
    <section class="connection-section">
      <h3>{{ t('nexus.recovery') }}</h3>
      <p class="hint">{{ t('nexus.recovery_help') }}</p>
      <button type="button" :disabled="!canReconcile" @click="recoveryGeneration = snapshot?.generation ?? ''">{{ t('nexus.reconcile') }}</button>
      <p v-if="!canReconcile && !busy" class="hint">{{ t(nexusErrorKey(snapshot?.capabilities.reconcileDisabledReason)) }}</p>
    </section>
    <ConfirmDialog v-if="recoveryGeneration" :prompt="t('nexus.confirm_recovery')" @cancel="recoveryGeneration = ''" @confirm="confirmRecovery">
      <p>{{ t('nexus.restart_warning') }}</p>
      <p>{{ origin }}</p>
    </ConfirmDialog>
  </section>
</template>
<style scoped>
.nexus-panel { min-width: 0; overflow-wrap: anywhere; }
.nexus-panel .status-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
@media (max-width: 560px) { .nexus-panel .status-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
