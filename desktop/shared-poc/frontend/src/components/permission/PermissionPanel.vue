<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { usePermissionStore } from '../../stores/permission'
import { useContractStore } from '../../stores/contract'
import { useI18n } from '../../i18n'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import PermissionPolicy from './PermissionPolicy.vue'
import ApprovalList from './ApprovalList.vue'
const store = usePermissionStore()
const contract = useContractStore()
const { t } = useI18n()
let timer: ReturnType<typeof setInterval> | undefined
function reconnect() { void store.refresh() }
function visible() { if (document.visibilityState === 'visible') reconnect() }
onMounted(async () => {
  window.addEventListener('focus', reconnect)
  window.addEventListener('online', reconnect)
  document.addEventListener('visibilitychange', visible)
  timer = setInterval(() => { if (!store.confirmation && document.visibilityState === 'visible') reconnect() }, 5000)
  await contract.load()
  await store.refresh()
})
onUnmounted(() => {
  clearInterval(timer)
  window.removeEventListener('focus', reconnect)
  window.removeEventListener('online', reconnect)
  document.removeEventListener('visibilitychange', visible)
  store.cancel()
})
</script>
<template>
  <section class="feature-stack" aria-labelledby="permission-title" :aria-busy="store.busy">
    <div class="panel">
      <div class="panel-heading">
        <h2 id="permission-title">{{ t('permission.title') }}</h2>
        <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
      </div>
      <p>{{ t('permission.boundary') }}</p>
      <p>{{ t('permission.retry') }}</p>
      <p v-if="!store.fresh" role="status">{{ t('permission.stale') }}</p>
      <p v-if="store.error" role="alert">{{ t('permission.error') }} ({{ store.error }})</p>
      <p v-if="store.completed" role="status">{{ t('permission.completed') }}</p>
      <p v-if="store.snapshot">{{ t('permission.revision') }}: {{ store.snapshot.state_revision }} / {{ store.snapshot.policy?.revision }}</p>
    </div>
    <PermissionPolicy v-if="store.snapshot?.policy" :policy="store.snapshot.policy" :disabled="!store.canMutate || !store.canInvoke('updatePolicy')" @save="store.begin('update_policy', undefined, $event)" />
    <ApprovalList :records="store.pending" :disabled="!store.canMutate" :history="false" @decide="(kind, record) => store.begin(kind, record)" />
    <ApprovalList :records="store.history" :disabled="true" :history="true" />
    <ConfirmDialog v-if="store.confirmation" :prompt="t('permission.confirm')" @confirm="store.confirm" @cancel="store.cancel">
      <p>{{ store.confirmation.mutation.kind }} · {{ store.confirmation.mutation.approval_id }}</p>
      <p v-if="store.confirmation.approval">{{ store.confirmation.approval.tool }} / {{ store.confirmation.approval.action }} · {{ store.confirmation.approval.scope }} · {{ store.confirmation.approval.reason }}</p>
      <p>{{ t('permission.revision') }}: {{ store.confirmation.mutation.policy_revision }} · {{ store.confirmation.mutation.approval_version }}</p>
      <p v-if="store.confirmation.mutation.policy">{{ t('permission.policy') }}: {{ store.confirmation.mutation.policy.settings.approval_policy.mode }} · {{ store.confirmation.mutation.policy.settings.approval_policy.granular }}</p>
      <p>{{ t('permission.expires') }}: {{ store.confirmation.challenge.expires_at }}</p>
      <p>{{ t('permission.confirm_scope') }}</p>
      <p>{{ t('permission.retry') }}</p>
    </ConfirmDialog>
  </section>
</template>
