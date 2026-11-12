<script setup lang="ts">
import type { MCPManagedServer } from '../../api/desktopApi'
import { mcpDisplayState } from '../../features/mcpLogic'
import { useI18n, type MessageKey } from '../../i18n'

defineProps<{
  servers: MCPManagedServer[]
  selectedName: string
  emptyKey: MessageKey
}>()

const emit = defineEmits<{ select: [name: string] }>()
const { t } = useI18n()

function statusLabel(server: MCPManagedServer) {
  return t((`mcp.status_${mcpDisplayState(server)}`) as MessageKey)
}

function transportLabel(server: MCPManagedServer) {
  return t(server.transport === 'stdio' ? 'mcp.transport_stdio' : 'mcp.transport_http')
}
</script>

<template>
  <div v-if="servers.length" class="mcp-server-list" role="list">
    <button
      v-for="server in servers"
      :key="server.name"
      type="button"
      class="mcp-server-row"
      :class="{ selected: selectedName === server.name }"
      :aria-pressed="selectedName === server.name"
      @click="emit('select', server.name)"
    >
      <span class="mcp-server-row-heading">
        <strong>{{ server.displayName || server.name }}</strong>
        <span class="state-pill" :data-state="mcpDisplayState(server)">{{ statusLabel(server) }}</span>
      </span>
      <span class="execution-meta">{{ server.description }}</span>
      <span class="mcp-server-row-meta">
        <span>{{ transportLabel(server) }}</span>
        <span v-if="server.sourceType === 'plugin'">{{ t('mcp.plugin_owner', { name: server.pluginName || '—' }) }}</span>
        <span v-if="server.observation.toolCount == null">{{ t('mcp.tools_unknown') }}</span>
        <span v-else>{{ t('mcp.tool_count', { count: server.observation.toolCount }) }}</span>
      </span>
    </button>
  </div>
  <div v-else class="empty-state">
    <p>{{ t(emptyKey) }}</p>
  </div>
</template>
