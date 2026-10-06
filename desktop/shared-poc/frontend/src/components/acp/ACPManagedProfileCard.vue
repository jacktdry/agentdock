<script setup lang="ts">
import type { ACPManagedProfile } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { useI18n } from '../../i18n'
import { safeMetadata } from '../../features/acpLogic'

const props = defineProps<{
  profile: ACPManagedProfile
  isDefault: boolean
  busy: boolean
  activeSessions: number
  canDetect: boolean
  canCheckUpdate: boolean
  canUpdateAdapter: boolean
  canManageSettings: boolean
}>()
const emit = defineEmits<{
  edit: []
  toggle: [enabled: boolean]
  makeDefault: []
  delete: []
  probe: []
  useDetected: []
  checkUpdate: []
  updateAdapter: []
}>()
const { t } = useI18n()

const text = (value: string | undefined) => safeMetadata(value) || t('common.unavailable')
const presetKey = () => {
  switch (props.profile.preset) {
  case 'codex': return 'acp.preset_codex'
  case 'antigravity': return 'acp.preset_antigravity'
  case 'legacy': return 'acp.preset_legacy'
  default: return 'acp.preset_custom'
  }
}
const blockedKey = () => {
  switch (props.profile.blockedReason) {
  case 'adapter_not_found': return 'acp.blocked_adapter_not_found'
  case 'version_unavailable': return 'acp.blocked_version_unavailable'
  case 'update_not_available': return 'acp.blocked_update_not_available'
  case 'shared_install_not_managed': return 'acp.blocked_shared_install'
  case 'update_target_not_next_owned': return 'acp.blocked_not_next_owned'
  case 'installed_version_unavailable': return 'acp.blocked_installed_version'
  case 'installed_version_invalid': return 'acp.blocked_installed_version'
  case 'trusted_release_unavailable': return 'acp.blocked_trusted_release'
  case 'trusted_release_invalid': return 'acp.blocked_trusted_release'
  case 'trusted_release_incomplete': return 'acp.blocked_trusted_release'
  case 'platform_unsupported': return 'acp.blocked_platform'
  case 'update_outcome_unknown': return 'acp.blocked_update_outcome_unknown'
  default: return ''
  }
}
const hasDifferentDetectedAdapter = () => !props.profile.protectedArgs && !!props.profile.detectedCommand && (
  props.profile.detectedCommand !== props.profile.configuredCommand ||
  JSON.stringify(props.profile.detectedArgs ?? []) !== JSON.stringify(props.profile.configuredArgs ?? [])
)
</script>

<template>
  <article class="managed-profile">
    <div class="managed-profile-heading">
      <div>
        <div class="managed-profile-title">
          <h4>{{ text(profile.displayName || profile.id) }}</h4>
          <span v-if="profile.enabled" class="state-pill" data-state="healthy">{{ t('common.enabled') }}</span>
          <span v-else class="state-pill">{{ t('common.disabled') }}</span>
          <span v-if="isDefault" class="state-pill" data-state="running">{{ t('acp.default_badge') }}</span>
        </div>
        <p class="execution-meta">{{ t(presetKey() as any) }} · {{ text(profile.id) }}</p>
      </div>
      <button type="button" :disabled="busy || !canManageSettings" :aria-label="t('acp.edit_named', { name: text(profile.displayName || profile.id) })" @click="emit('edit')">
        {{ t('acp.edit') }}
      </button>
    </div>

    <dl class="status-grid managed-profile-status">
      <div><dt>{{ t('acp.availability') }}</dt><dd>{{ t(('acp.availability_' + profile.availability) as any) }}</dd></div>
      <div><dt>{{ t('acp.installed_version') }}</dt><dd>{{ text(profile.installedVersion) }}</dd></div>
      <div v-if="profile.latestVersion"><dt>{{ t('acp.latest_version') }}</dt><dd>{{ text(profile.latestVersion) }}</dd></div>
      <div><dt>{{ t('acp.version_state') }}</dt><dd>{{ t(('acp.version_' + profile.versionState) as any) }}</dd></div>
      <div><dt>{{ t('acp.active_sessions') }}</dt><dd>{{ activeSessions }}</dd></div>
    </dl>

    <details v-if="profile.configuredCommand || profile.detectedCommand" class="adapter-paths">
      <summary>{{ t('acp.adapter_paths') }}</summary>
      <dl class="status-grid">
        <div v-if="profile.configuredCommand"><dt>{{ t('acp.configured_command') }}</dt><dd class="path">{{ text(profile.configuredCommand) }}</dd></div>
        <div v-if="profile.detectedCommand"><dt>{{ t('acp.detected_command') }}</dt><dd class="path">{{ text(profile.detectedCommand) }}</dd></div>
      </dl>
    </details>

    <p v-if="blockedKey()" class="field-hint">{{ t(blockedKey() as any) }}</p>

    <div class="actions">
      <button type="button" :disabled="busy || !canDetect || !profile.canDetect" @click="emit('probe')">
        {{ profile.availability === 'unknown' ? t('acp.detect') : t('acp.recheck') }}
      </button>
      <button v-if="hasDifferentDetectedAdapter()" type="button" :disabled="busy || !canManageSettings" @click="emit('useDetected')">
        {{ t('acp.use_detected') }}
      </button>
      <button type="button" :disabled="busy || !canCheckUpdate || profile.availability !== 'available'" @click="emit('checkUpdate')">
        {{ t('acp.check_update') }}
      </button>
      <button v-if="profile.canUpdate && profile.latestVersion && profile.updatePlan" type="button" :disabled="busy || !canUpdateAdapter" @click="emit('updateAdapter')">
        {{ t('acp.update_to', { version: profile.latestVersion }) }}
      </button>
      <button type="button" :disabled="busy || !canManageSettings || (profile.enabled && isDefault)" @click="emit('toggle', !profile.enabled)">
        {{ profile.enabled ? t('acp.disable') : t('acp.enable') }}
      </button>
      <button type="button" :disabled="busy || !canManageSettings || isDefault || !profile.enabled" @click="emit('makeDefault')">
        {{ t('acp.make_default') }}
      </button>
      <button type="button" class="danger-button" :disabled="busy || !canManageSettings || isDefault || activeSessions > 0" @click="emit('delete')">
        {{ t('acp.delete') }}
      </button>
    </div>
    <p v-if="profile.enabled && isDefault" class="field-hint">{{ t('acp.default_disable_hint') }}</p>
    <p v-else-if="activeSessions > 0" class="field-hint">{{ t('acp.delete_active_hint') }}</p>
  </article>
</template>
