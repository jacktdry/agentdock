<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'
import type { ACPLifecycleUpdate } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { useACPStore } from '../../stores/acp'
import { useContractStore } from '../../stores/contract'
import { useI18n } from '../../i18n'
import { safeMetadata, policyKeys, type LifecyclePolicy } from '../../features/acpLogic'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import OperationStatus from '../common/OperationStatus.vue'
import ACPProfileCard from './ACPProfileCard.vue'
import SharedMemoryCard from './SharedMemoryCard.vue'
const store = useACPStore()
const contract = useContractStore()
const { t } = useI18n()
type Confirmation = { operation: 'close'; profileId: string; sessionId: string } | { operation: 'updateLifecycle'; update: ACPLifecycleUpdate }
const confirmation = shallowRef<Confirmation | null>(null)
async function confirm() {
  const action = confirmation.value
  confirmation.value = null
  if (!action || !store.canInvoke(action.operation)) return
  if (action.operation === 'close') await store.close(action.profileId, action.sessionId)
  else await store.updateLifecycle(action.update)
}
onMounted(async () => { await contract.load(); await store.refresh() })
</script>
<template>
  <section class="feature-stack" aria-labelledby="acp-title" :aria-busy="store.busy">
    <div class="panel">
      <div class="panel-heading">
        <h2 id="acp-title">{{ t('acp.title') }}</h2>
        <button type="button" :disabled="store.busy || !store.canRead" @click="store.refresh">{{ t('common.refresh') }}</button>
      </div>
      <p role="status" aria-live="polite">{{ t(`acp.state_${store.state}`) }}</p>
      <p v-if="store.status?.defaultProfile">{{ t('acp.default_profile') }}: {{ safeMetadata(store.status.defaultProfile) || t('common.unavailable') }}</p>
      <OperationStatus :busy="store.busy" :error="store.error" :completed="store.completed" />
    </div>
    <ACPProfileCard v-for="profile in store.status?.profiles ?? []" :key="profile.profile.id" :profile="profile"
      :can-close="store.canInvoke('close')" :can-update="store.canInvoke('updateLifecycle')"
      @close="confirmation = { operation: 'close', profileId: profile.profile.id, sessionId: $event }"
      @update="confirmation = { operation: 'updateLifecycle', update: $event }" />
    <SharedMemoryCard v-if="store.status" :memory="store.status.memory" />
    <ConfirmDialog v-if="confirmation" :prompt="t(confirmation.operation === 'close' ? 'acp.confirm_close' : 'acp.confirm_policy')"
      @confirm="confirm" @cancel="confirmation = null">
      <p class="safe-url">{{ t('acp.session') }}: {{ safeMetadata(confirmation.operation === 'close' ? confirmation.sessionId : confirmation.update.sessionId) || t('common.unavailable') }}</p>
      <p v-if="confirmation.operation === 'updateLifecycle'">{{ t(policyKeys[confirmation.update.policy as LifecyclePolicy]) }}
        <span v-if="confirmation.update.idleCloseAfterMs"> · {{ t('acp.minutes', { count: confirmation.update.idleCloseAfterMs / 60000 }) }}</span>
      </p>
    </ConfirmDialog>
  </section>
</template>
