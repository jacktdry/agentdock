<script setup lang="ts">
import type { ACPProfileStatus, ACPLifecycleCounts, ACPLifecycleUpdate } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { useI18n, type MessageKey } from '../../i18n'
import { safeMetadata } from '../../features/acpLogic'
import { errorMessage } from '../../api/errorMessage'
import ACPSessionRow from './ACPSessionRow.vue'
const props = defineProps<{ profile: ACPProfileStatus; canClose: boolean; canUpdate: boolean }>()
const emit = defineEmits<{ close: [sessionId: string]; update: [update: ACPLifecycleUpdate] }>()
const { t } = useI18n()
const counts: [keyof ACPLifecycleCounts, MessageKey][] = [
  ['managed', 'acp.managed'], ['loaded', 'acp.loaded'], ['running', 'acp.running'], ['ready', 'acp.ready'],
  ['idleManagedIdle', 'acp.idle'], ['idleManagedEligible', 'acp.eligible'], ['closed', 'acp.closed'], ['autoCloseFailures', 'acp.failures'],
]
const text = (value: string | undefined) => safeMetadata(value) || t('common.unavailable')
const bytes = (value: number | null) => value == null ? t('common.unavailable') : t('acp.bytes', { count: value })
function resources(sessionId: string) { return props.profile.resources?.find(item => item.sessionId === sessionId) }
</script>
<template>
  <article class="panel acp-card">
    <h3>{{ text(profile.profile.displayName || profile.profile.id) }}</h3>
    <dl class="status-grid">
      <div><dt>{{ t('acp.kind') }}</dt><dd>{{ text(profile.profile.kind) }}</dd></div>
      <div><dt>{{ t('acp.source') }}</dt><dd>{{ text(profile.profile.source) }}</dd></div>
      <div><dt>{{ t('common.enabled') }}</dt><dd>{{ t(profile.profile.enabled ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.installed') }}</dt><dd>{{ profile.profile.installed == null ? t('common.unavailable') : t(profile.profile.installed ? 'common.yes' : 'common.no') }}</dd></div>
      <div v-if="profile.profile.packageName"><dt>{{ t('acp.package') }}</dt><dd>{{ text(profile.profile.packageName) }}</dd></div>
      <div v-if="profile.profile.latestVersion"><dt>{{ t('acp.version') }}</dt><dd>{{ text(profile.profile.latestVersion) }}</dd></div>
      <div><dt>{{ t('acp.version_state') }}</dt><dd>{{ text(profile.profile.versionState) }}</dd></div>
    </dl>
    <dl class="metric-grid"><div v-for="[field, label] in counts" :key="field"><dt>{{ t(label) }}</dt><dd>{{ profile.counts[field] }}</dd></div></dl>
    <p v-if="profile.error" class="error">{{ errorMessage(profile.error) }}</p>
    <h4>{{ t('acp.adapter') }}</h4>
    <dl v-if="profile.adapterProcess" class="status-grid">
      <div><dt>{{ t('acp.pid') }}</dt><dd>{{ profile.adapterProcess.rootPid }}</dd></div>
      <div><dt>{{ t('acp.status') }}</dt><dd>{{ text(profile.adapterProcess.state) }}</dd></div>
      <div><dt>{{ t('acp.alive') }}</dt><dd>{{ profile.adapterProcess.alive == null ? t('common.unavailable') : t(profile.adapterProcess.alive ? 'common.yes' : 'common.no') }}</dd></div>
      <div><dt>{{ t('acp.descendants') }}</dt><dd>{{ profile.adapterProcess.descendantCount ?? t('common.unavailable') }}</dd></div>
      <div><dt>{{ t('acp.rss') }}</dt><dd>{{ bytes(profile.adapterProcess.rssBytes) }}</dd></div>
      <div><dt>{{ t('acp.tree_rss') }}</dt><dd>{{ bytes(profile.adapterProcess.treeRssBytes) }}</dd></div>
    </dl>
    <p v-else>{{ t('common.unavailable') }}</p>
    <p v-if="profile.adapterProcess?.error" class="error">{{ t('acp.process_error') }}</p>
    <h4>{{ t('acp.sessions') }}</h4>
    <ul v-if="profile.sessions?.length" class="acp-sessions">
      <ACPSessionRow v-for="session in profile.sessions" :key="session.sessionId" :profile-id="profile.profile.id"
        :session="session" :resources="resources(session.sessionId)" :can-close="canClose && !profile.error" :can-update="canUpdate && !profile.error"
        @close="emit('close', $event)" @update="emit('update', $event)" />
    </ul>
    <p v-else>{{ t('acp.empty_sessions') }}</p>
  </article>
</template>
