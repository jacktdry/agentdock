<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'
import type { ACPLifecycleUpdate, ACPManagedProfile } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type { ACPProfileSettings } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopruntime/models'
import { useACPStore } from '../../stores/acp'
import { useContractStore } from '../../stores/contract'
import { useI18n } from '../../i18n'
import { safeMetadata, policyKeys, type LifecyclePolicy } from '../../features/acpLogic'
import ConfirmDialog from '../common/ConfirmDialog.vue'
import OperationStatus from '../common/OperationStatus.vue'
import ACPManagedProfileCard from './ACPManagedProfileCard.vue'
import ACPProfileCard from './ACPProfileCard.vue'
import ACPProfileEditor from './ACPProfileEditor.vue'
import SharedMemoryCard from './SharedMemoryCard.vue'

const store = useACPStore()
const contract = useContractStore()
const { t } = useI18n()

type RuntimeConfirmation =
  | { operation: 'close'; profileId: string; sessionId: string }
  | { operation: 'updateLifecycle'; update: ACPLifecycleUpdate }

const runtimeConfirmation = shallowRef<RuntimeConfirmation | null>(null)
const deleteTarget = shallowRef<ACPManagedProfile | null>(null)
const editorOpen = ref(false)
const editorProfile = shallowRef<ACPManagedProfile | null>(null)

async function confirmRuntime() {
  const action = runtimeConfirmation.value
  runtimeConfirmation.value = null
  if (!action || !store.canInvoke(action.operation)) return
  if (action.operation === 'close') await store.close(action.profileId, action.sessionId)
  else await store.updateLifecycle(action.update)
}

function openAdd() {
  editorProfile.value = null
  editorOpen.value = true
}

function openEdit(profile: ACPManagedProfile) {
  editorProfile.value = profile
  editorOpen.value = true
}

async function saveProfile(profile: ACPProfileSettings, originalId?: string) {
  if (await store.upsertProfile(profile, originalId)) editorOpen.value = false
}

async function confirmDelete() {
  const target = deleteTarget.value
  deleteTarget.value = null
  if (target) await store.removeProfile(target.id)
}

async function setGlobalEnabled(event: Event) {
  const input = event.target as HTMLInputElement
  const wanted = input.checked
  if (!(await store.setGlobalEnabled(wanted))) input.checked = store.settings?.enabled ?? false
}

onMounted(async () => {
  await contract.load()
  await store.refresh()
})
</script>

<template>
  <section class="feature-stack" aria-labelledby="acp-title" :aria-busy="store.busy">
    <div class="panel">
      <div class="panel-heading">
        <div>
          <h2 id="acp-title">{{ t('acp.title') }}</h2>
          <p class="execution-meta">{{ t('acp.manager_intro') }}</p>
        </div>
        <div class="actions">
          <button type="button" :disabled="store.busy || (!store.canRead && !store.canReadSettings)" @click="store.refresh">{{ t('common.refresh') }}</button>
          <button type="button" :disabled="store.busy || !store.canSaveSettings" @click="openAdd">{{ t('acp.add_profile') }}</button>
        </div>
      </div>

      <p role="status" aria-live="polite">{{ t(('acp.state_' + store.state) as any) }}</p>
      <dl class="status-grid">
        <div><dt>{{ t('acp.default_profile') }}</dt><dd>{{ safeMetadata(store.settings?.defaultProfile) || t('common.unavailable') }}</dd></div>
        <div><dt>{{ t('acp.configured_profiles') }}</dt><dd>{{ store.configuredProfiles.length }}</dd></div>
        <div><dt>{{ t('acp.active_sessions') }}</dt><dd>{{ store.activeSessions }}</dd></div>
        <div><dt>{{ t('acp.attention_required') }}</dt><dd>{{ store.attentionCount }}</dd></div>
      </dl>

      <label class="checkbox-label">
        <input :checked="store.settings?.enabled ?? false" type="checkbox"
          :disabled="store.busy || !store.canSaveSettings || (!(store.settings?.enabled) && !store.canEnableACP)"
          @change="setGlobalEnabled">
        {{ t('acp.global_enabled') }}
      </label>
      <p v-if="!store.settings?.enabled && !store.canEnableACP" class="field-hint">{{ t('acp.enable_requirements') }}</p>
      <p v-if="store.restartRequired" class="restart-notice" role="status">{{ t('acp.restart_required') }}</p>
      <OperationStatus :busy="store.busy" :error="store.error" :completed="store.completed" />
    </div>

    <div class="panel">
      <div class="panel-heading">
        <div>
          <h3>{{ t('acp.profiles') }}</h3>
          <p class="execution-meta">{{ t('acp.profiles_intro') }}</p>
        </div>
        <button type="button" :disabled="store.busy || !store.canSaveSettings" @click="openAdd">{{ t('acp.add_profile') }}</button>
      </div>

      <div v-if="store.configuredProfiles.length" class="managed-profile-list">
        <ACPManagedProfileCard v-for="profile in store.configuredProfiles" :key="profile.id"
          :profile="profile" :is-default="store.settings?.defaultProfile === profile.id"
          :busy="store.busy || !store.canSaveSettings" :active-sessions="store.activeSessionsFor(profile.id)"
          :can-detect="store.canProbeProfile"
          @edit="openEdit(profile)" @probe="store.probeProfile(profile.id)" @use-detected="store.useDetectedAdapter(profile.id)"
          @toggle="store.setProfileEnabled(profile.id, $event)"
          @make-default="store.makeDefault(profile.id)" @delete="deleteTarget = profile" />
      </div>
      <div v-else class="empty-state">
        <p>{{ t('acp.no_profiles') }}</p>
        <button type="button" :disabled="store.busy || !store.canSaveSettings" @click="openAdd">{{ t('acp.add_profile') }}</button>
      </div>
    </div>

    <section aria-labelledby="acp-runtime-title" class="feature-stack">
      <div class="panel">
        <div class="panel-heading">
          <div>
            <h3 id="acp-runtime-title">{{ t('acp.runtime') }}</h3>
            <p class="execution-meta">{{ t('acp.runtime_intro') }}</p>
          </div>
        </div>
      </div>
      <ACPProfileCard v-for="profile in store.status?.profiles ?? []" :key="profile.profile.id" :profile="profile"
        :can-close="store.canInvoke('close')" :can-update="store.canInvoke('updateLifecycle')"
        @close="runtimeConfirmation = { operation: 'close', profileId: profile.profile.id, sessionId: $event }"
        @update="runtimeConfirmation = { operation: 'updateLifecycle', update: $event }" />
      <div v-if="!store.status?.profiles?.length" class="panel"><p>{{ t('acp.runtime_empty') }}</p></div>
    </section>

    <section aria-labelledby="acp-diagnostics-title" class="feature-stack">
      <div class="panel"><h3 id="acp-diagnostics-title">{{ t('acp.diagnostics') }}</h3></div>
      <SharedMemoryCard v-if="store.status" :memory="store.status.memory" />
    </section>

    <ACPProfileEditor v-if="editorOpen" :profile="editorProfile" :busy="store.busy"
      :existing-ids="store.configuredProfiles.map(profile => profile.id)"
      @save="saveProfile" @cancel="editorOpen = false" />

    <ConfirmDialog v-if="deleteTarget" :prompt="t('acp.confirm_delete')" @confirm="confirmDelete" @cancel="deleteTarget = null">
      <p>{{ safeMetadata(deleteTarget.displayName || deleteTarget.id) }}</p>
      <p>{{ t('acp.delete_explainer') }}</p>
    </ConfirmDialog>

    <ConfirmDialog v-if="runtimeConfirmation"
      :prompt="t(runtimeConfirmation.operation === 'close' ? 'acp.confirm_close' : 'acp.confirm_policy')"
      @confirm="confirmRuntime" @cancel="runtimeConfirmation = null">
      <p class="safe-url">{{ t('acp.session') }}: {{ safeMetadata(runtimeConfirmation.operation === 'close' ? runtimeConfirmation.sessionId : runtimeConfirmation.update.sessionId) || t('common.unavailable') }}</p>
      <p v-if="runtimeConfirmation.operation === 'updateLifecycle'">{{ t(policyKeys[runtimeConfirmation.update.policy as LifecyclePolicy]) }}
        <span v-if="runtimeConfirmation.update.idleCloseAfterMs"> · {{ t('acp.minutes', { count: runtimeConfirmation.update.idleCloseAfterMs / 60000 }) }}</span>
      </p>
    </ConfirmDialog>
  </section>
</template>
