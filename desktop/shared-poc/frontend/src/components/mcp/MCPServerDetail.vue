<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import type { MCPManagedServer } from '../../api/desktopApi'
import { mcpDisplayState } from '../../features/mcpLogic'
import { useI18n, type MessageKey } from '../../i18n'
import { useMCPStore } from '../../stores/mcp'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import MCPEnvironmentBindings from './MCPEnvironmentBindings.vue'
import MCPOAuthStatus from './MCPOAuthStatus.vue'
import MCPToolSummaryList from './MCPToolSummaryList.vue'

const props = defineProps<{
  server: MCPManagedServer
  registryRevision: string
}>()
const emit = defineEmits<{
  edit: []
  removed: []
  back: []
}>()
const store = useMCPStore()
const { t } = useI18n()

type Confirmation = 'enable' | 'disable' | 'reconnect' | 'remove'
const confirmation = shallowRef<Confirmation | null>(null)

const pluginOwned = computed(() => props.server.sourceType === 'plugin')
const displayState = computed(() => mcpDisplayState(props.server))
const statusLabel = computed(() => t((`mcp.status_${displayState.value}`) as MessageKey))
const transportLabel = computed(() => t(props.server.transport === 'stdio' ? 'mcp.transport_stdio' : 'mcp.transport_http'))
const authLabel = computed(() => {
  const status = props.server.observation.authStatus || 'unknown'
  return t((`mcp.oauth_status_${status}`) as MessageKey)
})
const confirmationPrompt = computed(() => {
  switch (confirmation.value) {
    case 'enable':
      return props.server.enableRequiresEnvironmentConfirmation ? t('mcp.confirm_enable_reuse') : t('mcp.confirm_enable')
    case 'disable':
      return t('mcp.confirm_disable')
    case 'reconnect':
      return t('mcp.confirm_reconnect')
    case 'remove':
      return t('mcp.confirm_remove')
    default:
      return ''
  }
})

async function confirmAction() {
  const action = confirmation.value
  confirmation.value = null
  if (!action) return
  if (action === 'enable') {
    await store.setEnabled(
      props.server,
      true,
      props.server.enableRequiresEnvironmentConfirmation,
      props.registryRevision,
      props.server.generation,
    )
  } else if (action === 'disable') {
    await store.setEnabled(props.server, false, false, props.registryRevision, props.server.generation)
  } else if (action === 'reconnect') {
    await store.reconnect(props.server, props.registryRevision, props.server.generation)
  } else if (action === 'remove') {
    const result = await store.remove(props.server, props.registryRevision, props.server.generation)
    if (result && !result.error && !result.outcomeUnknown) emit('removed')
  }
}
</script>

<template>
  <article class="panel mcp-detail" :aria-labelledby="`mcp-detail-${server.name}`">
    <div class="panel-heading mcp-detail-heading">
      <div>
        <button type="button" class="mcp-back-button" @click="emit('back')">{{ t('mcp.back_to_servers') }}</button>
        <p class="eyebrow">{{ pluginOwned ? t('mcp.read_only_plugin') : t('mcp.detail_title') }}</p>
        <h2 :id="`mcp-detail-${server.name}`">{{ server.displayName || server.name }}</h2>
        <p class="execution-meta">{{ server.description }}</p>
      </div>
      <span class="state-pill" :data-state="displayState">{{ statusLabel }}</span>
    </div>

    <div v-if="pluginOwned" class="restart-notice">
      <strong>{{ t('mcp.read_only_plugin') }}</strong>
      <p>{{ t('mcp.plugin_hint') }}</p>
      <p v-if="server.pluginName">{{ t('mcp.plugin_owner', { name: server.pluginName }) }}</p>
    </div>

    <dl class="status-grid mcp-detail-grid">
      <div><dt>{{ t('mcp.source') }}</dt><dd>{{ pluginOwned ? t('mcp.source_plugin') : t('mcp.source_standalone') }}</dd></div>
      <div><dt>{{ t('mcp.transport_label') }}</dt><dd>{{ transportLabel }}</dd></div>
      <div><dt>{{ t('mcp.runtime_state') }}</dt><dd>{{ statusLabel }}</dd></div>
      <div><dt>{{ t('mcp.authorization_state') }}</dt><dd>{{ authLabel }}</dd></div>
    </dl>

    <section class="mcp-subsection" aria-labelledby="mcp-connection-title">
      <div class="section-heading">
        <div>
          <h3 id="mcp-connection-title">{{ t('mcp.connection_title') }}</h3>
          <p v-if="server.observation.observedAt" class="execution-meta">
            {{ t('mcp.last_observed') }} · {{ server.observation.observedAt }}
          </p>
          <p v-else class="execution-meta">{{ t('mcp.never_observed') }}</p>
        </div>
      </div>
      <dl class="mcp-config-grid">
        <div v-if="server.transport === 'streamable_http'">
          <dt>{{ t('mcp.endpoint') }}</dt>
          <dd class="safe-url">{{ server.urlProtected ? t('mcp.protected_value') : (server.url || t('common.unavailable')) }}</dd>
        </div>
        <div v-else>
          <dt>{{ t('mcp.command') }}</dt>
          <dd class="safe-url mono">{{ server.command || t('common.unavailable') }}</dd>
        </div>
        <div>
          <dt>{{ t('mcp.protocol_pin') }}</dt>
          <dd>{{ server.protocolVersion || t('mcp.default_value') }}</dd>
        </div>
        <div>
          <dt>{{ t('mcp.timeout') }}</dt>
          <dd>{{ server.timeoutMs }}</dd>
        </div>
      </dl>
    </section>

    <div v-if="!pluginOwned" class="mcp-detail-actions actions">
      <button type="button" :disabled="!store.canInvoke('update', server)" @click="emit('edit')">{{ t('mcp.edit') }}</button>
      <button
        v-if="!server.enabled"
        type="button"
        :disabled="!store.canInvoke('setEnabled', server)"
        @click="confirmation = 'enable'"
      >{{ t('mcp.enable') }}</button>
      <button
        v-else
        type="button"
        :disabled="!store.canInvoke('setEnabled', server)"
        @click="confirmation = 'disable'"
      >{{ t('mcp.disable') }}</button>
      <button type="button" :disabled="!store.canInvoke('reconnect', server) || !server.enabled" @click="confirmation = 'reconnect'">
        {{ t('mcp.reconnect') }}
      </button>
      <button type="button" class="danger-button" :disabled="!store.canInvoke('remove', server)" @click="confirmation = 'remove'">
        {{ t('mcp.remove') }}
      </button>
    </div>

    <p v-if="server.enableRequiresEnvironmentConfirmation" class="restart-notice">{{ t('mcp.environment_reuse_required') }}</p>

    <MCPEnvironmentBindings v-if="!pluginOwned" :server="server" :registry-revision="registryRevision" />
    <MCPOAuthStatus v-if="!pluginOwned && server.transport === 'streamable_http'" :server="server" :registry-revision="registryRevision" />
    <MCPToolSummaryList :server="server" :tools="store.toolsFor(server.name)" />

    <ConfirmDialog v-if="confirmation" :prompt="confirmationPrompt" @cancel="confirmation = null" @confirm="confirmAction">
      <p v-if="confirmation === 'enable'">{{ t('mcp.confirm_enable_hint') }}</p>
      <p v-else-if="confirmation === 'reconnect'">{{ t('mcp.reconnect_hint') }}</p>
      <p v-else-if="confirmation === 'remove'">{{ t('mcp.confirm_remove_hint') }}</p>
    </ConfirmDialog>
  </article>
</template>
