<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { errorMessage } from '../../api/errorMessage'
import { Domain, type RuntimeActionName } from '../../api/desktopApi'
import { useI18n, type MessageKey } from '../../i18n'
import { useContractStore } from '../../stores/contract'
import { useRuntimeStore } from '../../stores/runtime'

const { t } = useI18n()
const runtime = useRuntimeStore()
const contract = useContractStore()
const { status, runtimeRoot, loading, actionPending, error, announcement, stateKind, stateLabel } =
  storeToRefs(runtime)
const confirmationAction = ref<RuntimeActionName | null>(null)

const actionMessageKeys: Record<RuntimeActionName, MessageKey> = {
  start: 'common.start',
  restart: 'common.restart',
  stop: 'common.stop',
}

const confirmationLabel = computed(() => {
  if (!confirmationAction.value) return ''
  return t(actionMessageKeys[confirmationAction.value])
})

onMounted(() => {
  void contract.load()
  void runtime.refresh()
})

function canInvoke(action: RuntimeActionName) {
  return contract.canInvoke(Domain.DomainRuntime, action)
}

function requestAction(action: RuntimeActionName) {
  if (!canInvoke(action)) return
  if (loading.value || actionPending.value) return
  confirmationAction.value = action
}

async function confirmAction() {
  const action = confirmationAction.value
  confirmationAction.value = null
  if (!action) return
  await runtime.perform(action)
}

function cancelConfirmation() {
  confirmationAction.value = null
}
</script>

<template>
  <section class="panel" aria-labelledby="runtime-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ t('runtime.eyebrow') }}</p>
        <h2 id="runtime-title">{{ t('runtime.title') }}</h2>
      </div>
      <span class="state-pill" :data-state="stateKind">{{ stateLabel }}</span>
    </div>

    <dl class="status-grid">
      <div>
        <dt>{{ t('runtime.process') }}</dt>
        <dd>{{ status.running ? t('common.running') : t('common.stopped') }}</dd>
      </div>
      <div>
        <dt>{{ t('runtime.health') }}</dt>
        <dd>{{ status.healthy ? t('common.healthy') : t('common.not_healthy') }}</dd>
      </div>
      <div>
        <dt>{{ t('runtime.autostart') }}</dt>
        <dd>{{ status.startupEnabled ? t('common.enabled') : t('common.disabled') }}</dd>
      </div>
      <div>
        <dt>{{ t('runtime.nexus') }}</dt>
        <dd>{{ status.nexusConnected ? t('common.connected') : t('common.not_connected') }}</dd>
      </div>
    </dl>

    <p class="path" :title="runtimeRoot">{{ runtimeRoot || t('runtime.root_unavailable') }}</p>
    <p v-if="error" class="error" role="alert">{{ errorMessage(error) }}</p>

    <div class="actions">
      <button type="button" :disabled="loading || actionPending !== null" @click="runtime.refresh">
        {{ t('common.refresh') }}
      </button>
      <button
        type="button"
        :disabled="loading || actionPending !== null || !canInvoke('start')"
        @click="requestAction('start')"
      >
        {{ t('common.start') }}
      </button>
      <button
        type="button"
        :disabled="loading || actionPending !== null || !canInvoke('restart')"
        @click="requestAction('restart')"
      >
        {{ t('common.restart') }}
      </button>
      <button
        type="button"
        :disabled="loading || actionPending !== null || !canInvoke('stop')"
        @click="requestAction('stop')"
      >
        {{ t('common.stop') }}
      </button>
    </div>

    <div
      v-if="confirmationAction"
      class="confirmation-row"
      role="group"
      aria-labelledby="runtime-confirmation-label"
    >
      <p id="runtime-confirmation-label">
        {{ t('runtime.confirm_prompt', { action: confirmationLabel }) }}
      </p>
      <div class="actions">
        <button type="button" :disabled="actionPending !== null" @click="confirmAction">
          {{ t('runtime.confirm_action', { action: confirmationLabel }) }}
        </button>
        <button type="button" :disabled="actionPending !== null" @click="cancelConfirmation">
          {{ t('common.cancel') }}
        </button>
      </div>
    </div>

    <p class="sr-only" aria-live="polite">{{ announcement }}</p>
  </section>
</template>
