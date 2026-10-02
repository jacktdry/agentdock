<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'
import { useRuntimeStore } from '../../stores/runtime'

const runtime = useRuntimeStore()
const { status, runtimeRoot, loading, actionPending, error, announcement, stateLabel } =
  storeToRefs(runtime)

onMounted(() => {
  void runtime.refresh()
})
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
      <button type="button" :disabled="actionPending !== null" @click="runtime.perform('start')">
        Start
      </button>
      <button type="button" :disabled="actionPending !== null" @click="runtime.perform('restart')">
        Restart
      </button>
      <button type="button" :disabled="actionPending !== null" @click="runtime.perform('stop')">
        Stop
      </button>
    </div>

    <p class="sr-only" aria-live="polite">{{ announcement }}</p>
  </section>
</template>
