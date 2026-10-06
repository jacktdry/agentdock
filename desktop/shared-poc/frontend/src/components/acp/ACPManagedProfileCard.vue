<script setup lang="ts">
import type { ACPManagedProfile } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import { useI18n } from '../../i18n'
import { safeMetadata } from '../../features/acpLogic'

const props = defineProps<{
  profile: ACPManagedProfile
  isDefault: boolean
  busy: boolean
  activeSessions: number
}>()
const emit = defineEmits<{
  edit: []
  toggle: [enabled: boolean]
  makeDefault: []
  delete: []
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
      <button type="button" :disabled="busy" :aria-label="t('acp.edit_named', { name: text(profile.displayName || profile.id) })" @click="emit('edit')">
        {{ t('acp.edit') }}
      </button>
    </div>

    <dl class="status-grid managed-profile-status">
      <div><dt>{{ t('acp.availability') }}</dt><dd>{{ t(('acp.availability_' + profile.availability) as any) }}</dd></div>
      <div><dt>{{ t('acp.version_state') }}</dt><dd>{{ t(('acp.version_' + profile.versionState) as any) }}</dd></div>
      <div><dt>{{ t('acp.active_sessions') }}</dt><dd>{{ activeSessions }}</dd></div>
      <div><dt>{{ t('acp.source') }}</dt><dd>{{ text(profile.source) }}</dd></div>
    </dl>

    <p v-if="profile.blockedReason" class="execution-meta">{{ t('acp.adapter_actions_pending') }}</p>

    <div class="actions">
      <button type="button" :disabled="busy || (profile.enabled && isDefault)" @click="emit('toggle', !profile.enabled)">
        {{ profile.enabled ? t('acp.disable') : t('acp.enable') }}
      </button>
      <button type="button" :disabled="busy || isDefault || !profile.enabled" @click="emit('makeDefault')">
        {{ t('acp.make_default') }}
      </button>
      <button type="button" class="danger-button" :disabled="busy || isDefault || activeSessions > 0" @click="emit('delete')">
        {{ t('acp.delete') }}
      </button>
    </div>
    <p v-if="profile.enabled && isDefault" class="field-hint">{{ t('acp.default_disable_hint') }}</p>
    <p v-else-if="activeSessions > 0" class="field-hint">{{ t('acp.delete_active_hint') }}</p>
  </article>
</template>
