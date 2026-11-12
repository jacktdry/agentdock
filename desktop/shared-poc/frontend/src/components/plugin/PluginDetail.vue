<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import type { PluginManagedItem } from '../../api/desktopApi'
import { shortFingerprint } from '../../features/pluginLogic'
import { useI18n } from '../../i18n'
import { usePluginStore } from '../../stores/plugin'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import PluginEnvironmentBindings from './PluginEnvironmentBindings.vue'
import PluginRemoveDialog from './PluginRemoveDialog.vue'

const props = defineProps<{ plugin: PluginManagedItem }>()
const emit = defineEmits<{ back: []; removed: []; openMcp: [] }>()
const store = usePluginStore()
const { t } = useI18n()

type Confirmation = 'enable' | 'disable' | 'purge'
const confirmation = shallowRef<Confirmation | null>(null)
const removeOpen = ref(false)

const detail = computed(() => store.detailFor(props.plugin.name))
const mcpComponents = computed(() => detail.value?.mcp ?? [])
const warnings = computed(() => detail.value?.warnings ?? [])
const hasLocalProcess = computed(() => mcpComponents.value.some((component) => component.transport === 'stdio' || !!component.command))
const hasRemote = computed(() => mcpComponents.value.some((component) => component.transport !== 'stdio' && !!component.endpoint))
const statusLabel = computed(() => props.plugin.recoveryState
  ? t('plugin.status_recovery')
  : props.plugin.enabled ? t('plugin.status_enabled') : t('plugin.status_disabled'))

async function loadDetail() {
  await store.inspect(props.plugin)
}

async function confirmLifecycle() {
  const action = confirmation.value
  confirmation.value = null
  if (action === 'enable') await store.setEnabled(props.plugin, true)
  else if (action === 'disable') await store.setEnabled(props.plugin, false)
  else if (action === 'purge') {
    const result = await store.removePurge(props.plugin)
    if (result?.completed && !result.error && !result.outcomeUnknown) emit('removed')
  }
}

async function removeKeep() {
  removeOpen.value = false
  const result = await store.removeKeep(props.plugin)
  if (result?.completed && !result.error && !result.outcomeUnknown) emit('removed')
}

function choosePurge() {
  removeOpen.value = false
  confirmation.value = 'purge'
}

async function chooseUpdate(sourceType: 'folder' | 'zip') {
  await store.chooseCandidate('update', sourceType, props.plugin)
}

watch(() => props.plugin.generation, () => void loadDetail())
onMounted(() => void loadDetail())
</script>

<template>
  <article class="panel plugin-detail" :aria-labelledby="'plugin-detail-' + plugin.name">
    <div class="panel-heading plugin-detail-heading">
      <div>
        <button type="button" class="plugin-back-button" @click="emit('back')">{{ t('plugin.back_to_plugins') }}</button>
        <p class="eyebrow">{{ t('plugin.detail_title') }}</p>
        <h2 :id="'plugin-detail-' + plugin.name">{{ plugin.name }}</h2>
        <p v-if="plugin.description" class="execution-meta">{{ plugin.description }}</p>
      </div>
      <span class="state-pill" :data-state="plugin.recoveryState ? 'needs_attention' : plugin.enabled ? 'available' : 'disabled'">
        {{ statusLabel }}
      </span>
    </div>

    <div v-if="plugin.recoveryState" class="restart-notice" role="status">
      <strong>{{ t('plugin.cleanup_required') }}</strong>
      <p>{{ t('plugin.cleanup_required_hint') }}</p>
    </div>

    <dl class="status-grid plugin-detail-grid">
      <div><dt>{{ t('plugin.version') }}</dt><dd>{{ plugin.version }}</dd></div>
      <div><dt>{{ t('plugin.format') }}</dt><dd>{{ plugin.format }}</dd></div>
      <div><dt>{{ t('plugin.skills') }}</dt><dd>{{ plugin.skillsCount }}</dd></div>
      <div><dt>{{ t('plugin.mcp_components') }}</dt><dd>{{ plugin.mcpCount }}</dd></div>
      <div><dt>{{ t('plugin.installed_at') }}</dt><dd>{{ plugin.installedAt || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('plugin.fingerprint') }}</dt><dd class="mono">{{ shortFingerprint(plugin.packageFingerprint) }}</dd></div>
    </dl>

    <p v-if="plugin.provenance?.origin" class="hint">
      {{ t('plugin.provenance') }}: {{ plugin.provenance.origin }}
      <span v-if="plugin.provenance.ref"> · {{ plugin.provenance.ref }}</span>
      <span v-if="plugin.provenance.revision"> · {{ plugin.provenance.revision }}</span>
    </p>

    <div class="actions plugin-detail-actions">
      <button
        v-if="!plugin.enabled"
        type="button"
        :disabled="!store.canInvoke('setEnabled') || !!plugin.recoveryState"
        @click="confirmation = 'enable'"
      >{{ t('plugin.enable') }}</button>
      <button
        v-else
        type="button"
        :disabled="!store.canInvoke('setEnabled') || !!plugin.recoveryState"
        @click="confirmation = 'disable'"
      >{{ t('plugin.disable') }}</button>
      <button type="button" :disabled="!store.canInvoke('chooseCandidate') || !!plugin.recoveryState" @click="chooseUpdate('folder')">
        {{ t('plugin.update_folder') }}
      </button>
      <button type="button" :disabled="!store.canInvoke('chooseCandidate') || !!plugin.recoveryState" @click="chooseUpdate('zip')">
        {{ t('plugin.update_zip') }}
      </button>
      <button type="button" :disabled="store.busy" @click="emit('openMcp')">{{ t('plugin.open_mcp') }}</button>
      <button type="button" class="danger-button" :disabled="store.busy || !!plugin.recoveryState" @click="removeOpen = true">
        {{ t('plugin.remove') }}
      </button>
    </div>

    <section v-if="warnings.length" class="plugin-subsection" aria-labelledby="plugin-warning-title">
      <h3 id="plugin-warning-title">{{ t('plugin.warnings') }}</h3>
      <ul class="plugin-review-list">
        <li v-for="warning in warnings" :key="warning">{{ warning }}</li>
      </ul>
    </section>

    <section class="plugin-subsection" aria-labelledby="plugin-mcp-title">
      <div class="section-heading">
        <div>
          <h3 id="plugin-mcp-title">{{ t('plugin.mcp_settings') }}</h3>
          <p class="field-hint">{{ t('plugin.mcp_settings_hint') }}</p>
        </div>
      </div>

      <div v-if="mcpComponents.length" class="plugin-component-list">
        <article v-for="component in mcpComponents" :key="component.name" class="plugin-component-card">
          <div class="plugin-component-heading">
            <strong>{{ component.name }}</strong>
            <span>{{ component.transport }}</span>
          </div>
          <p v-if="component.description" class="hint">{{ component.description }}</p>
          <p v-if="component.endpoint" class="mono plugin-safe-value">{{ component.endpoint }}</p>
          <p v-if="component.command" class="mono plugin-safe-value">{{ component.command }}</p>
          <PluginEnvironmentBindings :plugin="plugin" :component="component" />
        </article>
      </div>
      <p v-else-if="detail" class="empty-state">{{ t('plugin.no_mcp') }}</p>
      <p v-else class="hint">{{ t('common.loading') }}</p>
    </section>

    <ConfirmDialog v-if="confirmation === 'enable'" :prompt="t('plugin.confirm_enable')" @cancel="confirmation = null" @confirm="confirmLifecycle">
      <p>{{ t('plugin.confirm_enable_hint') }}</p>
      <p v-if="hasLocalProcess" class="restart-notice">{{ t('plugin.enable_local_process_notice') }}</p>
      <p v-if="hasRemote" class="restart-notice">{{ t('plugin.enable_remote_notice') }}</p>
    </ConfirmDialog>
    <ConfirmDialog v-if="confirmation === 'disable'" :prompt="t('plugin.confirm_disable')" @cancel="confirmation = null" @confirm="confirmLifecycle">
      <p>{{ t('plugin.confirm_disable_hint') }}</p>
    </ConfirmDialog>
    <ConfirmDialog v-if="confirmation === 'purge'" :prompt="t('plugin.confirm_purge')" @cancel="confirmation = null" @confirm="confirmLifecycle">
      <p class="error">{{ t('plugin.confirm_purge_hint') }}</p>
    </ConfirmDialog>
    <PluginRemoveDialog
      v-if="removeOpen"
      :plugin-name="plugin.name"
      :busy="store.busy"
      @cancel="removeOpen = false"
      @keep="removeKeep"
      @purge="choosePurge"
    />
  </article>
</template>
