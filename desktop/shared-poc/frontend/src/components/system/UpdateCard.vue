<script setup lang="ts">
import { useUpdateStore } from '../../stores/update'
import { useI18n } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
const store = useUpdateStore()
const { t } = useI18n()
</script>
<template>
  <section class="panel" aria-labelledby="update-title">
    <h2 id="update-title">{{ t('update.title') }}</h2>
    <dl v-if="store.data" class="status-grid">
      <div><dt>{{ t('update.current') }}</dt><dd>{{ store.data.currentVersion || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('update.latest') }}</dt><dd>{{ store.data.latestVersion || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('update.desktop_current') }}</dt><dd>{{ store.data.desktopCurrentVersion || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('update.core_available') }}</dt><dd>{{ t(store.data.updateAvailable ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('update.desktop_available') }}</dt><dd>{{ t(store.data.desktopUpdateAvailable ? 'common.yes' : 'common.no') }}</dd></div>
    </dl>
    <p v-else>{{ t('update.not_checked') }}</p>
    <p class="hint">{{ t('update.native_only') }}</p>
    <button type="button" :disabled="store.busy" @click="store.refresh">{{ t('update.check') }}</button>
    <OperationStatus :busy="store.busy" :error="store.error" :completed="!!store.data" />
  </section>
</template>
