import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as ACPService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/acpservice'
import type {
  ACPLifecycleUpdate,
  ACPManagedProfile,
  ACPManagerSnapshot,
  ACPMutationResult,
  ACPSettingsMutationResult,
  ACPStatusResult,
  APIError,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type { ACPProfileSettings } from '../../bindings/github.com/uvwt/agentdock/internal/desktopruntime/models'
import { Domain, clientError } from '../api/desktopApi'
import { useContractStore } from './contract'

function editableProfile(profile: ACPManagedProfile): ACPProfileSettings {
  return {
    id: profile.id,
    displayName: profile.displayName,
    kind: profile.runtimeKind,
    command: profile.configuredCommand,
    args: [...(profile.configuredArgs ?? [])],
    enabled: profile.enabled,
  }
}

export const useACPStore = defineStore('acp', () => {
  const contract = useContractStore()
  const status = ref<ACPStatusResult | null>(null)
  const settings = ref<ACPManagerSnapshot | null>(null)
  const loading = ref(false)
  const pending = ref(false)
  const statusError = ref<APIError | null>(null)
  const settingsError = ref<APIError | null>(null)
  const mutationError = ref<APIError | null>(null)
  const completed = ref(false)
  const restartRequired = ref(false)

  const busy = computed(() => loading.value || pending.value)
  const error = computed(() => mutationError.value ?? settingsError.value ?? statusError.value)
  const canRead = computed(() => contract.canInvoke(Domain.DomainACP, 'status'))
  const canReadSettings = computed(() => contract.canInvoke(Domain.DomainACP, 'settings'))
  const canProbeProfile = computed(() => contract.canInvoke(Domain.DomainACP, 'probeProfile'))
  const canSaveSettings = computed(() => contract.canInvoke(Domain.DomainACP, 'saveSettings'))
  const state = computed(() => {
    if (loading.value && !settings.value) return 'loading'
    if (!canReadSettings.value || settingsError.value || !settings.value) return 'unavailable'
    if (!settings.value.enabled) return 'disabled'
    if (!(settings.value.profiles?.length)) return 'empty'
    return 'ready'
  })
  const configuredProfiles = computed(() => settings.value?.profiles ?? [])
  const activeSessions = computed(() => (status.value?.profiles ?? []).reduce(
    (total, profile) => total + (profile.sessions ?? []).filter(session => !session.closedAt).length,
    0,
  ))
  const attentionCount = computed(() => {
    let count = (status.value?.profiles ?? []).filter(profile => profile.error || profile.counts.autoCloseFailures > 0).length
    if (status.value?.memory?.error || status.value?.memory?.healthy === false) count += 1
    return count
  })
  const canEnableACP = computed(() => {
    const snapshot = settings.value
    if (!snapshot) return false
    const profiles = snapshot.profiles ?? []
    return profiles.some(profile => profile.enabled && profile.id === snapshot.defaultProfile && !!profile.configuredCommand)
  })

  function canInvoke(operation: 'close' | 'updateLifecycle') {
    return !busy.value && !!status.value?.enabled && contract.canInvoke(Domain.DomainACP, operation)
  }

  function activeSessionsFor(profileId: string) {
    const profile = status.value?.profiles?.find(item => item.profile.id === profileId)
    return (profile?.sessions ?? []).filter(session => !session.closedAt).length
  }

  async function readStatus() {
    if (!canRead.value) return
    try {
      const result = await ACPService.Status()
      statusError.value = result.error ?? null
      if (!statusError.value) status.value = result
    } catch {
      statusError.value = clientError('acp_status_failed', '')
    }
  }

  async function readSettings() {
    if (!canReadSettings.value) return
    try {
      const result = await ACPService.Settings()
      settingsError.value = result.error ?? null
      if (!settingsError.value) settings.value = result
    } catch {
      settingsError.value = clientError('acp_settings_read_failed', '')
    }
  }

  async function refresh() {
    if (busy.value || (!canRead.value && !canReadSettings.value)) return
    loading.value = true
    statusError.value = null
    settingsError.value = null
    mutationError.value = null
    completed.value = false
    try {
      await Promise.all([readSettings(), readStatus()])
    } finally {
      loading.value = false
    }
  }

  async function mutate(operation: 'close' | 'updateLifecycle', call: () => Promise<ACPMutationResult>) {
    if (!canInvoke(operation)) return
    pending.value = true
    mutationError.value = null
    completed.value = false
    try {
      const result = await call()
      mutationError.value = result.error ?? (result.completed ? null : clientError('acp_mutation_incomplete', ''))
      if (!mutationError.value) {
        completed.value = true
        if (canRead.value) await readStatus()
      }
    } catch {
      mutationError.value = clientError('acp_mutation_failed', '')
    } finally {
      pending.value = false
    }
  }

  async function probeProfile(profileId: string) {
    if (pending.value || !settings.value || !canProbeProfile.value) return false
    pending.value = true
    mutationError.value = null
    completed.value = false
    try {
      let result
      try {
        result = await ACPService.ProbeProfile(profileId)
      } catch {
        mutationError.value = clientError('acp_profile_probe_failed', '')
        return false
      }
      mutationError.value = result.error ?? null
      if (mutationError.value || !result.profile) return false
      settings.value = {
        ...settings.value,
        profiles: (settings.value.profiles ?? []).map(profile => profile.id === profileId ? result.profile! : profile),
      }
      return true
    } finally {
      pending.value = false
    }
  }

  async function useDetectedAdapter(profileId: string) {
    const profile = settings.value?.profiles?.find(item => item.id === profileId)
    if (!profile?.detectedCommand) return false
    return upsertProfile({
      ...editableProfile(profile),
      command: profile.detectedCommand,
      args: [...(profile.detectedArgs ?? [])],
    }, profileId)
  }

  async function saveConfiguration(enabled: boolean, defaultProfile: string, profiles: ACPProfileSettings[]) {
    if (pending.value || !settings.value || !canSaveSettings.value) return false
    pending.value = true
    mutationError.value = null
    completed.value = false
    restartRequired.value = false
    try {
      let result: ACPSettingsMutationResult
      try {
        result = await ACPService.SaveSettings(settings.value.revision, enabled, defaultProfile, profiles)
      } catch {
        mutationError.value = clientError('acp_settings_save_failed', '')
        return false
      }
      mutationError.value = result.error ?? (result.completed && result.persisted ? null : clientError('acp_settings_save_incomplete', ''))
      if (mutationError.value || !result.snapshot) return false
      settings.value = result.snapshot
      completed.value = true
      restartRequired.value = result.restartRequired
      return true
    } finally {
      pending.value = false
    }
  }

  function editableProfiles() {
    return configuredProfiles.value.map(editableProfile)
  }

  async function upsertProfile(profile: ACPProfileSettings, originalId?: string) {
    const profiles = editableProfiles()
    if (originalId) {
      const index = profiles.findIndex(item => item.id === originalId)
      if (index < 0) return false
      profiles[index] = profile
    } else {
      profiles.push(profile)
    }
    return saveConfiguration(settings.value?.enabled ?? false, settings.value?.defaultProfile ?? '', profiles)
  }

  async function setProfileEnabled(profileId: string, enabled: boolean) {
    const snapshot = settings.value
    if (!snapshot || (!enabled && snapshot.defaultProfile === profileId)) return false
    const profiles = editableProfiles()
    const profile = profiles.find(item => item.id === profileId)
    if (!profile) return false
    profile.enabled = enabled
    return saveConfiguration(snapshot.enabled, snapshot.defaultProfile, profiles)
  }

  async function makeDefault(profileId: string) {
    const snapshot = settings.value
    const profile = snapshot?.profiles?.find(item => item.id === profileId)
    if (!snapshot || !profile?.enabled) return false
    return saveConfiguration(snapshot.enabled, profileId, editableProfiles())
  }

  async function removeProfile(profileId: string) {
    const snapshot = settings.value
    if (!snapshot || snapshot.defaultProfile === profileId || activeSessionsFor(profileId) > 0) return false
    return saveConfiguration(snapshot.enabled, snapshot.defaultProfile, editableProfiles().filter(item => item.id !== profileId))
  }

  async function setGlobalEnabled(enabled: boolean) {
    const snapshot = settings.value
    if (!snapshot || (enabled && !canEnableACP.value)) return false
    return saveConfiguration(enabled, snapshot.defaultProfile, editableProfiles())
  }

  const close = (profileId: string, sessionId: string) => mutate('close', () => ACPService.Close(profileId, sessionId))
  const updateLifecycle = (update: ACPLifecycleUpdate) => mutate('updateLifecycle', () => ACPService.UpdateLifecycle(update))

  return {
    status,
    settings,
    loading,
    error,
    completed,
    restartRequired,
    busy,
    canRead,
    canReadSettings,
    canProbeProfile,
    canSaveSettings,
    canEnableACP,
    state,
    configuredProfiles,
    activeSessions,
    attentionCount,
    canInvoke,
    activeSessionsFor,
    refresh,
    probeProfile,
    useDetectedAdapter,
    upsertProfile,
    setProfileEnabled,
    makeDefault,
    removeProfile,
    setGlobalEnabled,
    close,
    updateLifecycle,
  }
})
