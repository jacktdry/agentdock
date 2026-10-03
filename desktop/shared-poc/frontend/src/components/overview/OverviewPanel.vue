<script setup lang="ts">
import { onMounted } from 'vue'
import type { Section } from '../../navigation'
import { useRuntimeStore } from '../../stores/runtime'
import { useConnectionStore } from '../../stores/connection'
import { useUpdateStore } from '../../stores/update'
import { useDiagnosticsStore } from '../../stores/diagnostics'
import { useI18n } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
const emit = defineEmits<{ navigate: [section: Section] }>()
const runtime = useRuntimeStore()
const connection = useConnectionStore()
const update = useUpdateStore()
const diagnostics = useDiagnosticsStore()
const { t } = useI18n()
onMounted(() => {
  void runtime.refresh(); void connection.refresh(); void diagnostics.refresh()
  if (!update.data) void update.refresh()
})
</script>
<template>
  <section aria-labelledby="overview-title">
    <h2 id="overview-title">{{ t('nav.overview') }}</h2>
    <div class="overview-grid">
      <article class="panel">
        <h3>{{ t('runtime.title') }}</h3>
        <p class="state-pill" :data-state="runtime.stateKind">{{ runtime.stateLabel }}</p>
        <OperationStatus :busy="runtime.loading" :error="runtime.error" />
        <button type="button" @click="emit('navigate', 'runtime')">{{ t('overview.open_runtime') }}</button>
      </article>
      <article class="panel">
        <h3>{{ t('connection.title') }}</h3>
        <p>{{ connection.status ? t(connection.status.running ? 'common.running' : 'common.stopped') : t('common.unavailable') }}</p>
        <p class="safe-url">{{ connection.publicURL || t('common.unavailable') }}</p>
        <OperationStatus :busy="connection.busy" :error="connection.error" />
        <button type="button" @click="emit('navigate', 'connection')">{{ t('overview.open_connection') }}</button>
      </article>
      <article class="panel">
        <h3>{{ t('update.title') }}</h3>
        <p>{{ !update.data ? t('update.not_checked') : t(update.data.updateAvailable || update.data.desktopUpdateAvailable ? 'update.available' : 'update.current_state') }}</p>
        <OperationStatus :busy="update.busy" :error="update.error" />
        <button type="button" @click="emit('navigate', 'system')">{{ t('overview.open_system') }}</button>
      </article>
      <article class="panel">
        <h3>{{ t('diagnostics.title') }}</h3>
        <p>{{ diagnostics.data ? `${diagnostics.data.platform} / ${diagnostics.data.architecture}` : t('common.unavailable') }}</p>
        <OperationStatus :busy="diagnostics.busy" :error="diagnostics.error" />
        <button type="button" @click="emit('navigate', 'system')">{{ t('overview.open_diagnostics') }}</button>
      </article>
    </div>
  </section>
</template>
