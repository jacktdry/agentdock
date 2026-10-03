<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'
import { Domain, type BasicSettings } from '../../api/desktopApi'
import { logLevels } from '../../features/settingsLogic'
import { useBasicSettingsStore } from '../../stores/basicSettings'
import { useContractStore } from '../../stores/contract'
import { useI18n, type MessageKey } from '../../i18n'
import OperationStatus from '../common/OperationStatus.vue'
import ConfirmDialog from '../common/ConfirmDialog.vue'
const store = useBasicSettingsStore()
const contract = useContractStore()
const { t } = useI18n()
const confirmation = shallowRef<BasicSettings | null>(null)
const logKeys: Record<string, MessageKey> = {
  debug: 'basicsettings.log_debug', info: 'basicsettings.log_info', warn: 'basicsettings.log_warn', error: 'basicsettings.log_error',
}
function requestSave() {
  if (store.valid && !store.busy && contract.canInvoke(Domain.DomainSettings, 'save')) confirmation.value = store.payload()
}
async function confirm() {
  const settings = confirmation.value
  confirmation.value = null
  if (settings && contract.canInvoke(Domain.DomainSettings, 'save')) await store.save(settings)
}
onMounted(() => { void contract.load(); void store.load() })
</script>
<template>
  <section class="panel" aria-labelledby="basic-settings-title">
    <h2 id="basic-settings-title">{{ t('basicsettings.title') }}</h2>
    <p class="hint">{{ t('basicsettings.scope') }}</p>
    <form class="settings-form" @submit.prevent="requestSave">
      <fieldset :disabled="store.busy || !store.current || !!confirmation">
        <label for="basic-port">{{ t('basicsettings.port') }}</label>
        <input id="basic-port" v-model.number="store.draft.port" type="number" min="1" max="65535" step="1" required aria-describedby="settings-validation" />
        <label for="basic-log-level">{{ t('basicsettings.log_level') }}</label>
        <select id="basic-log-level" v-model="store.draft.logLevel">
          <option v-for="level in logLevels" :key="level" :value="level">{{ t(logKeys[level]) }}</option>
        </select>
        <label class="checkbox-label" for="basic-autostart">
          <input id="basic-autostart" v-model="store.draft.coreAutostart" type="checkbox" :disabled="!store.mutable" aria-describedby="autostart-hint" />
          {{ t('basicsettings.autostart') }}
        </label>
        <p v-if="store.current && !store.mutable" id="autostart-hint" class="hint">{{ t('basicsettings.native_autostart') }}</p>
        <p id="settings-validation" class="hint">{{ t('basicsettings.validation') }}</p>
        <div class="actions">
          <button type="button" @click="store.reset">{{ t('common.reset') }}</button>
          <button type="submit" :disabled="!store.valid || !contract.canInvoke(Domain.DomainSettings, 'save')">{{ t('common.save') }}</button>
        </div>
      </fieldset>
    </form>
    <button type="button" :disabled="store.busy || !!confirmation" @click="store.load">{{ t('common.refresh') }}</button>
    <OperationStatus :busy="store.busy" :error="store.error" :completed="store.completed" />
    <ConfirmDialog v-if="confirmation" :prompt="t('basicsettings.confirm')" @confirm="confirm" @cancel="confirmation = null">
      <dl class="confirmation-values">
        <dt>{{ t('basicsettings.port') }}</dt><dd>{{ confirmation.port }}</dd>
        <dt>{{ t('basicsettings.log_level') }}</dt><dd>{{ t(logKeys[confirmation.logLevel]) }}</dd>
        <dt>{{ t('basicsettings.autostart') }}</dt><dd>{{ t(confirmation.coreAutostart ? 'common.enabled' : 'common.disabled') }}</dd>
      </dl>
      <p class="hint">{{ t('basicsettings.restart_note') }}</p>
    </ConfirmDialog>
  </section>
</template>
