<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { Domain, type RuntimeActionName } from '../../api/desktopApi'
import { useContractStore } from '../../stores/contract'
import { useRuntimeStore } from '../../stores/runtime'

const runtime = useRuntimeStore()
const contract = useContractStore()
const { status, runtimeRoot, loading, actionPending, error, announcement, stateLabel } =
  storeToRefs(runtime)
const confirmationAction = ref<RuntimeActionName | null>(null)

const confirmationLabel = computed(() => {
  if (!confirmationAction.value) return ''
  return confirmationAction.value.charAt(0).toUpperCase() + confirmationAction.value.slice(1)
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
  if (contract.requiresConfirmation(Domain.DomainRuntime, action)) {
    confirmationAction.value = action
    return
  }
  void runtime.perform(action)
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
        <p class="eyebrow">Core lifecycle bridge</p>
        <h2 id="runtime-title">Runtime</h2>
      </div>
      <span class="state-pill" :data-state="stateLabel.toLowerCase()">{{ stateLabel }}</span>
    </div>

    <dl class="status-grid">
      <div>
        <dt>Process</dt>
        <dd>{{ status.running ? 'Running' : 'Stopped' }}</dd>
      </div>
      <div>
        <dt>Health</dt>
        <dd>{{ status.healthy ? 'Healthy' : 'Not healthy' }}</dd>
      </div>
      <div>
        <dt>Autostart</dt>
        <dd>{{ status.startupEnabled ? 'Enabled' : 'Disabled' }}</dd>
      </div>
      <div>
        <dt>Nexus</dt>
        <dd>{{ status.nexusConnected ? 'Connected' : 'Not connected' }}</dd>
      </div>
    </dl>

    <p class="path" :title="runtimeRoot">{{ runtimeRoot || 'Runtime root unavailable' }}</p>
    <p v-if="error" class="error" role="alert">{{ error.code }} — {{ error.message }}</p>

    <div class="actions">
      <button type="button" :disabled="loading || actionPending !== null" @click="runtime.refresh">
        Refresh
      </button>
      <button
        type="button"
        :disabled="actionPending !== null || !canInvoke('start')"
        @click="requestAction('start')"
      >
        Start
      </button>
      <button
        type="button"
        :disabled="actionPending !== null || !canInvoke('restart')"
        @click="requestAction('restart')"
      >
        Restart
      </button>
      <button
        type="button"
        :disabled="actionPending !== null || !canInvoke('stop')"
        @click="requestAction('stop')"
      >
        Stop
      </button>
    </div>

    <div
      v-if="confirmationAction"
      class="confirmation-row"
      role="group"
      aria-labelledby="runtime-confirmation-label"
    >
      <p id="runtime-confirmation-label">
        Confirm {{ confirmationLabel.toLowerCase() }} of the AgentDock runtime?
      </p>
      <div class="actions">
        <button type="button" :disabled="actionPending !== null" @click="confirmAction">
          Confirm {{ confirmationLabel }}
        </button>
        <button type="button" :disabled="actionPending !== null" @click="cancelConfirmation">
          Cancel
        </button>
      </div>
    </div>

    <p class="sr-only" aria-live="polite">{{ announcement }}</p>
  </section>
</template>
