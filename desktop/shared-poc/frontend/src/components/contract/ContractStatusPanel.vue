<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'
import { useContractStore } from '../../stores/contract'

const contract = useContractStore()
const { manifest, negotiated, loading, error, unavailableCount, experimentalCount } =
  storeToRefs(contract)

onMounted(() => {
  void contract.load()
})
</script>

<template>
  <section class="panel" aria-labelledby="contract-title">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">Control-plane contract</p>
        <h2 id="contract-title">Desktop API</h2>
      </div>
      <span class="state-pill" :data-state="error ? 'unavailable' : 'healthy'">
        {{ loading ? 'Negotiating' : error ? 'Rejected' : 'v' + (manifest?.protocolVersion ?? '—') }}
      </span>
    </div>

    <dl class="status-grid contract-metrics">
      <div>
        <dt>Negotiated</dt>
        <dd>{{ negotiated.length }}</dd>
      </div>
      <div>
        <dt>Experimental</dt>
        <dd>{{ experimentalCount }}</dd>
      </div>
      <div>
        <dt>Unavailable</dt>
        <dd>{{ unavailableCount }}</dd>
      </div>
    </dl>

    <p class="hint">
      Unavailable domains are advertised explicitly instead of returning empty placeholder data.
    </p>
    <p v-if="error" class="error" role="alert">
      {{ error.code }} — {{ error.message }}
    </p>
  </section>
</template>
