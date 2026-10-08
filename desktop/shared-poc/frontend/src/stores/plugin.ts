import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  Domain,
  clientError,
  desktopApi,
  type APIError,
  type PluginCandidate,
  type PluginDetail,
  type PluginManagedItem,
  type PluginOperationResult,
  type PluginRecoveryItem,
  type PluginManagerSnapshot,
} from '../api/desktopApi'
import { newPluginRequestID, pluginNeedsAttention } from '../features/pluginLogic'
import { useContractStore } from './contract'

interface PluginFeedback {
  requestId: string
  outcome: string
  outcomeUnknown: boolean
  runtimeImpact: string
  recoveryRequired: boolean
  error: APIError | null
}

function environmentKey(name: string, component: string) {
  return name + '\u0000' + component
}

export const usePluginStore = defineStore('plugin', () => {
  const contract = useContractStore()
  const snapshot = ref<PluginManagerSnapshot | null>(null)
  const stale = ref(false)
  const loading = ref(false)
  const pending = ref(false)
  const readError = ref<APIError | null>(null)
  const mutationError = ref<APIError | null>(null)
  const lastFeedback = ref<PluginFeedback | null>(null)
  const lastOperation = ref<PluginOperationResult | null>(null)
  const details = ref<Record<string, PluginDetail>>({})
  const environments = ref<Record<string, PluginOperationResult>>({})
  const candidate = ref<PluginCandidate | null>(null)
  const candidateSourceLabel = ref('')

  const plugins = computed(() => snapshot.value?.plugins ?? [])
  const canRead = computed(() => contract.canInvoke(Domain.DomainPlugin, 'snapshot'))
  const busy = computed(() => loading.value || pending.value)
  const enabledCount = computed(() => plugins.value.filter((plugin) => plugin.enabled).length)
  const attentionCount = computed(() => plugins.value.filter(pluginNeedsAttention).length)

  function canInvoke(operation: string) {
    return !busy.value && !stale.value && snapshot.value !== null && contract.canInvoke(Domain.DomainPlugin, operation)
  }

  function resetMutationState() {
    mutationError.value = null
    lastFeedback.value = null
  }

  function clearPluginCachedState(name: string) {
    delete details.value[name]
    for (const key of Object.keys(environments.value)) {
      if (key.startsWith(name + '\u0000')) delete environments.value[key]
    }
  }

  function syncSnapshot(next: PluginManagerSnapshot) {
    const previous = new Map((snapshot.value?.plugins ?? []).map((plugin) => [plugin.name, plugin.generation]))
    const current = new Map((next.plugins ?? []).map((plugin) => [plugin.name, plugin.generation]))
    snapshot.value = next
    stale.value = false
    readError.value = null
    for (const [name, generation] of previous) {
      if (current.get(name) !== generation) clearPluginCachedState(name)
    }
    for (const name of Object.keys(details.value)) {
      if (!current.has(name)) clearPluginCachedState(name)
    }
  }

  async function readSnapshot() {
    try {
      const result = await desktopApi.pluginSnapshot()
      if (result.error || !result.snapshot.authoritative) {
        readError.value = result.error ?? clientError('plugin_snapshot_not_authoritative', '')
        stale.value = snapshot.value !== null
        return false
      }
      syncSnapshot(result.snapshot)
      return true
    } catch (caught) {
      readError.value = clientError('plugin_snapshot_failed', caught)
      stale.value = snapshot.value !== null
      return false
    }
  }

  async function refresh() {
    if (busy.value || !canRead.value) return false
    loading.value = true
    resetMutationState()
    try {
      return await readSnapshot()
    } finally {
      loading.value = false
    }
  }

  function feedbackFrom(result: PluginOperationResult): PluginFeedback {
    return {
      requestId: result.requestId ?? '',
      outcome: result.outcome ?? '',
      outcomeUnknown: result.outcomeUnknown ?? false,
      runtimeImpact: result.runtimeImpact ?? '',
      recoveryRequired: result.recoveryRequired ?? false,
      error: result.error ?? null,
    }
  }

  async function reconcileOperation(requestId: string) {
    if (!requestId || !contract.canInvoke(Domain.DomainPlugin, 'operationStatus')) return null
    try {
      const result = await desktopApi.pluginOperationStatus(requestId)
      lastOperation.value = result
      if (result.found && !result.pending && !result.outcomeUnknown) await readSnapshot()
      return result
    } catch {
      lastOperation.value = null
      return null
    }
  }

  async function runMutation(operation: string, call: (requestId: string) => Promise<PluginOperationResult>) {
    if (!canInvoke(operation)) return null
    const requestId = newPluginRequestID()
    pending.value = true
    resetMutationState()
    try {
      let result: PluginOperationResult
      try {
        result = await call(requestId)
      } catch (caught) {
        mutationError.value = clientError('plugin_mutation_failed', caught)
        await reconcileOperation(requestId)
        await readSnapshot()
        return null
      }
      lastFeedback.value = feedbackFrom(result)
      mutationError.value = result.error ?? null
      if (result.outcomeUnknown) await reconcileOperation(result.requestId || requestId)
      await readSnapshot()
      return result
    } finally {
      pending.value = false
    }
  }

  async function inspect(plugin: PluginManagedItem) {
    if (stale.value || !snapshot.value || !contract.canInvoke(Domain.DomainPlugin, 'inspect')) return null
    try {
      const result = await desktopApi.pluginInspect(plugin.name)
      if (result.error) {
        mutationError.value = result.error
        return result
      }
      if (result.registryRevision !== snapshot.value.registryRevision || !result.detail || result.detail.plugin.generation !== plugin.generation) {
        await readSnapshot()
        return result
      }
      details.value[plugin.name] = result.detail
      return result
    } catch (caught) {
      mutationError.value = clientError('plugin_inspect_failed', caught)
      return null
    }
  }

  async function discardCandidate() {
    const current = candidate.value
    if (!current) return true
    if (!contract.canInvoke(Domain.DomainPlugin, 'discardCandidate')) return false
    try {
      const result = await desktopApi.pluginDiscardCandidate(current.candidateId)
      if (result.error) {
        mutationError.value = result.error
        return false
      }
      candidate.value = null
      candidateSourceLabel.value = ''
      return true
    } catch (caught) {
      mutationError.value = clientError('plugin_candidate_discard_failed', caught)
      return false
    }
  }

  async function chooseCandidate(kind: 'install' | 'update', sourceType: 'folder' | 'zip', target?: PluginManagedItem | null) {
    if (!canInvoke('chooseCandidate')) return null
    if (candidate.value && !(await discardCandidate())) return null
    mutationError.value = null
    pending.value = true
    try {
      const result = await desktopApi.pluginChooseCandidate({
        kind, sourceType,
        ...(target ? { targetName: target.name, targetGeneration: target.generation } : {}),
      })
      if (result.cancelled) return result
      if (result.error || !result.candidate) {
        mutationError.value = result.error ?? clientError('plugin_candidate_invalid', '')
        return result
      }
      candidate.value = result.candidate
      candidateSourceLabel.value = result.sourceLabel ?? ''
      return result
    } catch (caught) {
      mutationError.value = clientError('plugin_candidate_failed', caught)
      return null
    } finally {
      pending.value = false
    }
  }

  async function installCandidate() {
    const current = candidate.value
    const revision = snapshot.value?.registryRevision ?? ''
    if (!current || current.kind !== 'install' || !revision) return null
    const result = await runMutation('installCandidate', (requestId) => desktopApi.pluginInstallCandidate({
      requestId, candidateId: current.candidateId, name: current.review.name, expectedRegistryRevision: revision,
    }))
    if (result?.completed && !result.error && !result.outcomeUnknown) {
      candidate.value = null
      candidateSourceLabel.value = ''
    }
    return result
  }

  async function updateCandidate(plugin: PluginManagedItem) {
    const current = candidate.value
    const revision = snapshot.value?.registryRevision ?? ''
    if (!current || current.kind !== 'update' || current.targetName !== plugin.name || !revision) return null
    const result = await runMutation('updateCandidate', (requestId) => desktopApi.pluginUpdateCandidate({
      requestId, candidateId: current.candidateId, name: plugin.name, expectedRegistryRevision: revision, expectedGeneration: plugin.generation,
    }))
    if (result?.completed && !result.error && !result.outcomeUnknown) {
      candidate.value = null
      candidateSourceLabel.value = ''
    }
    return result
  }

  function mutationInput(plugin: PluginManagedItem, requestId: string) {
    return {
      requestId, name: plugin.name,
      expectedRegistryRevision: snapshot.value?.registryRevision ?? '',
      expectedGeneration: plugin.generation,
    }
  }

  async function setEnabled(plugin: PluginManagedItem, enabled: boolean) {
    return runMutation('setEnabled', (requestId) => desktopApi.pluginSetEnabled(mutationInput(plugin, requestId), enabled))
  }

  async function removeKeep(plugin: PluginManagedItem) {
    const result = await runMutation('removeKeep', (requestId) => desktopApi.pluginRemoveKeep(mutationInput(plugin, requestId)))
    if (result?.completed && !result.error) clearPluginCachedState(plugin.name)
    return result
  }

  async function removePurge(plugin: PluginManagedItem) {
    const result = await runMutation('removePurge', (requestId) => desktopApi.pluginRemovePurge(mutationInput(plugin, requestId)))
    if (result?.completed && !result.error) clearPluginCachedState(plugin.name)
    return result
  }

  async function retryPurge(recovery: PluginRecoveryItem) {
    const revision = snapshot.value?.registryRevision ?? ''
    if (!revision) return null
    return runMutation('removePurge', (requestId) => desktopApi.pluginRemovePurge({
      requestId,
      name: recovery.name,
      expectedRegistryRevision: revision,
      expectedGeneration: recovery.generation,
    }))
  }

  async function readEnvironment(plugin: PluginManagedItem, component: string) {
    if (stale.value || !contract.canInvoke(Domain.DomainPlugin, 'environment')) return null
    try {
      const result = await desktopApi.pluginEnvironment(plugin.name, component)
      mutationError.value = result.error ?? null
      if (!result.error) environments.value[environmentKey(plugin.name, component)] = result
      return result
    } catch (caught) {
      mutationError.value = clientError('plugin_environment_failed', caught)
      return null
    }
  }

  async function setEnvironment(plugin: PluginManagedItem, component: string, key: string, value: string) {
    const environment = environments.value[environmentKey(plugin.name, component)]
    if (!environment?.envRevision) return null
    const result = await runMutation('setEnvironment', (requestId) => desktopApi.pluginSetEnvironment({
      ...mutationInput(plugin, requestId), component, key, value, expectedEnvRevision: environment.envRevision ?? '',
    }))
    if (result && !result.error && !result.outcomeUnknown) environments.value[environmentKey(plugin.name, component)] = result
    return result
  }

  async function unsetEnvironment(plugin: PluginManagedItem, component: string, key: string) {
    const environment = environments.value[environmentKey(plugin.name, component)]
    if (!environment?.envRevision) return null
    const result = await runMutation('unsetEnvironment', (requestId) => desktopApi.pluginUnsetEnvironment({
      ...mutationInput(plugin, requestId), component, key, expectedEnvRevision: environment.envRevision ?? '',
    }))
    if (result && !result.error && !result.outcomeUnknown) environments.value[environmentKey(plugin.name, component)] = result
    return result
  }

  function detailFor(name: string) { return details.value[name] ?? null }
  function environmentFor(name: string, component: string) { return environments.value[environmentKey(name, component)] ?? null }

  function clearTransient() {
    mutationError.value = null
    lastFeedback.value = null
    lastOperation.value = null
  }

  return {
    snapshot, stale, loading, pending, busy, readError, mutationError, lastFeedback, lastOperation,
    details, environments, candidate, candidateSourceLabel, plugins, enabledCount, attentionCount, canRead, canInvoke,
    refresh, inspect, chooseCandidate, discardCandidate, installCandidate, updateCandidate, setEnabled, removeKeep, removePurge, retryPurge,
    readEnvironment, setEnvironment, unsetEnvironment, reconcileOperation, detailFor, environmentFor, clearTransient,
  }
})
