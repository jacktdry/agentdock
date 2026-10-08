<script setup lang="ts">
import type { PluginManagedItem } from '../../api/desktopApi'
import { useI18n } from '../../i18n'

defineProps<{ plugins: PluginManagedItem[]; selectedName: string }>()
const emit = defineEmits<{ select: [name: string] }>()
const { t } = useI18n()
</script>

<template>
  <div v-if="plugins.length" class="plugin-list">
    <button
      v-for="plugin in plugins"
      :key="plugin.name + ':' + plugin.generation"
      type="button"
      class="plugin-row"
      :class="{ selected: selectedName === plugin.name }"
      :aria-pressed="selectedName === plugin.name"
      @click="emit('select', plugin.name)"
    >
      <span class="plugin-row-heading">
        <strong>{{ plugin.name }}</strong>
        <span class="state-pill" :data-state="plugin.recoveryState ? 'needs_attention' : plugin.enabled ? 'available' : 'disabled'">
          {{ plugin.recoveryState ? t('plugin.status_recovery') : plugin.enabled ? t('plugin.status_enabled') : t('plugin.status_disabled') }}
        </span>
      </span>
      <span class="plugin-row-meta">
        <span>{{ plugin.version }} · {{ plugin.format }}</span>
        <span>{{ t('plugin.row_components', { skills: plugin.skillsCount, mcp: plugin.mcpCount }) }}</span>
      </span>
      <span v-if="plugin.warningCount" class="plugin-row-warning">{{ t('plugin.row_warnings', { count: plugin.warningCount }) }}</span>
    </button>
  </div>
  <p v-else class="empty-state">{{ t('plugin.empty') }}</p>
</template>
