<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { onMounted } from 'vue'
import { useI18n } from '../../i18n'
import { useContractStore } from '../../stores/contract'

const { t } = useI18n()
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
        <p class="eyebrow">{{ t('contract.eyebrow') }}</p>
        <h2 id="contract-title">{{ t('contract.title') }}</h2>
      </div>
      <span class="state-pill" :data-state="error ? 'unavailable' : 'healthy'">
        {{
          loading
            ? t('contract.negotiating')
            : error
              ? t('common.rejected')
              : t('contract.version', { version: manifest?.protocolVersion ?? '—' })
        }}
      </span>
    </div>

    <dl class="status-grid contract-metrics">
      <div>
        <dt>{{ t('common.negotiated') }}</dt>
        <dd>{{ negotiated.length }}</dd>
      </div>
      <div>
        <dt>{{ t('common.experimental') }}</dt>
        <dd>{{ experimentalCount }}</dd>
      </div>
      <div>
        <dt>{{ t('common.unavailable') }}</dt>
        <dd>{{ unavailableCount }}</dd>
      </div>
    </dl>

    <p class="hint">{{ t('contract.hint') }}</p>
    <p v-if="error" class="error" role="alert">
      {{ error.code }} — {{ error.message }}
    </p>
  </section>
</template>
