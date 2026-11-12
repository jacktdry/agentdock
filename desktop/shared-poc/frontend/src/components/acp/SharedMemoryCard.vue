<script setup lang="ts">
import type { ACPMemoryStatus } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { useI18n, type MessageKey } from '../../i18n'
import { safeEndpoint, safeMetadata } from '../../features/acpLogic'
import { errorMessage } from '../../api/errorMessage'
defineProps<{ memory: ACPMemoryStatus }>()
const { t } = useI18n()
const fields: [keyof ACPMemoryStatus, MessageKey][] = [
  ['version', 'acp.version'], ['backend', 'acp.backend'], ['provider', 'acp.provider'],
  ['storagePath', 'acp.storage'], ['totalMemories', 'acp.total_memories'], ['protocolVersion', 'acp.protocol'],
]
function value(value: unknown) { return typeof value === 'number' ? value : typeof value === 'string' ? safeMetadata(value) || t('common.unavailable') : t('common.unavailable') }
</script>
<template>
  <section class="panel acp-card" aria-labelledby="acp-memory-title">
    <h3 id="acp-memory-title">{{ t('acp.memory') }}</h3>
    <dl class="status-grid">
      <div><dt>{{ t('acp.endpoint') }}</dt><dd>{{ safeEndpoint(memory.endpoint) || t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('acp.status') }}</dt><dd>{{ t(memory.healthy ? 'common.healthy' : 'acp.unhealthy') }}</dd></div>
      <div v-for="[field, label] in fields" :key="field"><dt>{{ t(label) }}</dt><dd>{{ value(memory[field]) }}</dd></div>
    </dl>
    <p v-if="memory.error" class="error">{{ errorMessage(memory.error) }}</p>
  </section>
</template>
