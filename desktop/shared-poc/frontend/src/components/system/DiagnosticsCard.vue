<script setup lang="ts">
import { onMounted } from 'vue'
import { useDiagnosticsStore } from '../../stores/diagnostics'
import { useI18n } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
const store = useDiagnosticsStore()
const { t } = useI18n()
onMounted(() => { void store.refresh() })
</script>
<template>
  <section class="panel" aria-labelledby="diagnostics-title">
    <h2 id="diagnostics-title">{{ t('diagnostics.title') }}</h2>
    <p class="hint">{{ t('diagnostics.scope') }}</p>
    <dl v-if="store.data" class="status-grid">
      <div><dt>{{ t('diagnostics.platform') }}</dt><dd>{{ store.data.platform }}</dd></div>
      <div><dt>{{ t('diagnostics.architecture') }}</dt><dd>{{ store.data.architecture }}</dd></div>
      <div><dt>{{ t('diagnostics.runtime_available') }}</dt><dd>{{ t(store.data.runtimeDirectoryAvailable ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('diagnostics.manifest_available') }}</dt><dd>{{ t(store.data.manifestAvailable ? 'common.yes' : 'common.no') }}</dd></div>
    </dl>
    <p v-if="store.data" class="path" :title="store.data.runtimeRoot">{{ store.data.runtimeRoot }}</p>
    <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('common.refresh') }}</button>
    <OperationStatus :busy="store.busy" :error="store.error" :completed="!!store.data" />
  </section>
</template>
