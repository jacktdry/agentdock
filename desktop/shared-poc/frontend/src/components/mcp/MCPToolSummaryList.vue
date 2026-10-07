<script setup lang="ts">
import type { MCPManagedServer, MCPToolSummary } from '../../api/desktopApi'
import { useI18n } from '../../i18n'

const props = defineProps<{ server: MCPManagedServer; tools: MCPToolSummary[] }>()
const { t } = useI18n()
</script>

<template>
  <section class="mcp-subsection" aria-labelledby="mcp-tools-title">
    <div class="section-heading">
      <h3 id="mcp-tools-title">{{ t('mcp.known_tools') }}</h3>
      <span v-if="server.observation.toolCount != null" class="state-pill">
        {{ t('mcp.tool_count', { count: server.observation.toolCount }) }}
      </span>
    </div>
    <p v-if="server.observation.toolCount == null" class="field-hint">{{ t('mcp.tools_unknown') }}</p>
    <p v-else-if="server.observation.toolCount === 0" class="field-hint">{{ t('mcp.tools_empty') }}</p>
    <ul v-else-if="tools.length" class="mcp-tool-list">
      <li v-for="tool in tools" :key="tool.qualifiedName || tool.name">
        <strong class="mono">{{ tool.name }}</strong>
      </li>
    </ul>
    <p v-else class="field-hint">{{ t('mcp.tool_count', { count: server.observation.toolCount }) }}</p>
  </section>
</template>
