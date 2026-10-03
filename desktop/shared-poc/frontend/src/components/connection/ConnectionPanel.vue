<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { Domain, type ConnectionActionName } from '../../api/desktopApi'
import { useConnectionStore } from '../../stores/connection'
import { useContractStore } from '../../stores/contract'
import { useI18n, type MessageKey } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
import ConfirmDialog from '../common/ConfirmDialog.vue'
const store = useConnectionStore()
const contract = useContractStore()
const { t } = useI18n()
const confirmation = shallowRef<ConnectionActionName | null>(null)
const actions: ConnectionActionName[] = ['start', 'stop', 'restart', 'regenerate']
const actionKeys: Record<ConnectionActionName, MessageKey> = {
  start: 'common.start', stop: 'common.stop', restart: 'common.restart', regenerate: 'connection.regenerate',
}
const modeKeys: Record<string, MessageKey> = {
  quick: 'connection.mode_quick', named: 'connection.mode_named', tailscale: 'connection.mode_tailscale', none: 'connection.mode_none',
}
const modeLabel = computed(() => t(modeKeys[store.status?.mode ?? ''] ?? 'common.unavailable'))
function allowed(action: ConnectionActionName) {
  return store.canPerform(action) && contract.canInvoke(Domain.DomainConnection, action)
}
async function confirm() {
  const action = confirmation.value
  confirmation.value = null
  if (action && allowed(action)) await store.perform(action)
}
onMounted(() => { void contract.load(); void store.refresh() })
</script>
<template>
  <section class="panel" aria-labelledby="connection-title">
    <h2 id="connection-title">{{ t('connection.title') }}</h2>
    <dl v-if="store.status" class="status-grid">
      <div><dt>{{ t('connection.mode') }}</dt><dd>{{ modeLabel }}</dd></div>
      <div><dt>{{ t('runtime.process') }}</dt><dd>{{ t(store.status.running ? 'common.running' : 'common.stopped') }}</dd></div>
      <div><dt>{{ t('connection.ready') }}</dt><dd>{{ t(store.status.ready ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('runtime.autostart') }}</dt><dd>{{ t(store.status.startupEnabled ? 'common.enabled' : 'common.disabled') }}</dd></div>
    </dl>
    <p v-else>{{ t('common.unavailable') }}</p>
    <p>{{ t('connection.public_url') }}: <span class="safe-url">{{ store.publicURL || t('common.unavailable') }}</span></p>
    <p class="hint">{{ t('connection.native_only') }}</p>
    <p id="regenerate-hint" class="hint">{{ t('connection.quick_only') }}</p>
    <div class="actions">
      <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
      <button v-for="action in actions" :key="action" type="button" :disabled="!allowed(action)"
        :aria-describedby="action === 'regenerate' ? 'regenerate-hint' : undefined" @click="confirmation = action">
        {{ t(actionKeys[action]) }}
      </button>
    </div>
    <OperationStatus :busy="store.busy" :error="store.error" :completed="store.completed" />
    <ConfirmDialog v-if="confirmation" :prompt="t('connection.confirm', { action: t(actionKeys[confirmation]) })"
      @confirm="confirm" @cancel="confirmation = null" />
  </section>
</template>
