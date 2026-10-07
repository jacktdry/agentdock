import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  Domain,
  clientError,
  desktopApi,
  type APIError,
  type MCPAuthCallback,
  type MCPAuthorizationStatus,
  type MCPConfigInput,
  type MCPEnvironmentResult,
  type MCPManagedServer,
  type MCPOperationStatusResult,
  type MCPSnapshot,
  type MCPToolSummary,
} from '../api/desktopApi'
import { isMCPAttention, isTerminalOAuthStatus, newMCPRequestID } from '../features/mcpLogic'
import { useContractStore } from './contract'

interface MCPFeedback {
  requestId: string
  outcome: string
  outcomeUnknown: boolean
  runtimeImpact: string
  reconnectRequired: boolean
  recoveryRequired: boolean
  error: APIError | null
}

const oauthPollDelayMs = 2000
const maxOAuthPolls = 150

export const useMCPStore = defineStore('mcp', () => {
  const contract = useContractStore()
  const snapshot = ref<MCPSnapshot | null>(null)
  const stale = ref(false)
  const loading = ref(false)
  const pending = ref(false)
  const readError = ref<APIError | null>(null)
  const mutationError = ref<APIError | null>(null)
  const lastFeedback = ref<MCPFeedback | null>(null)
  const lastOperation = ref<MCPOperationStatusResult | null>(null)
  const environments = ref<Record<string, MCPEnvironmentResult>>({})
  const knownTools = ref<Record<string, MCPToolSummary[]>>({})
  const authorization = ref<MCPAuthorizationStatus | null>(null)
  const callbackOptions = ref<MCPAuthCallback[]>([])

  let oauthTimer: ReturnType<typeof setTimeout> | undefined
  let oauthPollToken = 0

  const servers = computed(() => snapshot.value?.servers ?? [])
  const canRead = computed(() => contract.canInvoke(Domain.DomainMCP, 'snapshot'))
  const busy = computed(() => loading.value || pending.value)
  const availableCount = computed(() => servers.value.filter((server) => server.observation.status === 'ready' && !server.observation.stale).length)
  const attentionCount = computed(() => servers.value.filter(isMCPAttention).length)
  const authorizationRequiredCount = computed(() => servers.value.filter((server) => server.observation.authStatus === 'auth_required').length)

  function canInvoke(operation: string, server?: MCPManagedServer | null) {
    if (busy.value || stale.value || !snapshot.value || !contract.canInvoke(Domain.DomainMCP, operation)) return false
    if (server?.sourceType === 'plugin') return false
    return true
  }

  function resetError() {
    mutationError.value = null
    lastFeedback.value = null
  }

  function syncSnapshot(next: MCPSnapshot) {
    const previous = new Map((snapshot.value?.servers ?? []).map((server) => [server.name, server.generation]))
    snapshot.value = next
    stale.value = false
    readError.value = null

    const current = new Map((next.servers ?? []).map((server) => [server.name, server.generation]))
    for (const [name, generation] of previous) {
      if (current.get(name) !== generation) {
        delete knownTools.value[name]
        delete environments.value[name]
      }
    }
    for (const name of Object.keys(knownTools.value)) {
      if (!current.has(name)) delete knownTools.value[name]
    }
  }

  async function readSnapshot() {
    try {
      const result = await desktopApi.mcpSnapshot()
      if (result.error) {
        readError.value = result.error
        stale.value = snapshot.value !== null
        return false
      }
      syncSnapshot(result.snapshot)
      return true
    } catch {
      readError.value = clientError('mcp_snapshot_failed', '')
      stale.value = snapshot.value !== null
      return false
    }
  }

  async function refresh() {
    if (busy.value || !canRead.value) return false
    loading.value = true
    resetError()
    try {
      return await readSnapshot()
    } finally {
      loading.value = false
    }
  }

  function feedbackFrom(result: {
    requestId?: string
    outcome?: string
    outcomeUnknown?: boolean
    runtimeImpact?: string
    reconnectRequired?: boolean
    recoveryRequired?: boolean
    error?: APIError | null
  }): MCPFeedback {
    return {
      requestId: result.requestId ?? '',
      outcome: result.outcome ?? '',
      outcomeUnknown: result.outcomeUnknown ?? false,
      runtimeImpact: result.runtimeImpact ?? '',
      reconnectRequired: result.reconnectRequired ?? false,
      recoveryRequired: result.recoveryRequired ?? false,
      error: result.error ?? null,
    }
  }

  async function reconcileOperation(requestId: string) {
    if (!requestId || !contract.canInvoke(Domain.DomainMCP, 'operationStatus')) return null
    try {
      const result = await desktopApi.mcpOperationStatus(requestId)
      lastOperation.value = result
      if (result.found && !result.pending && !result.outcomeUnknown) {
        await readSnapshot()
      }
      return result
    } catch {
      lastOperation.value = null
      return null
    }
  }

  async function runMutation<T extends {
    requestId?: string
    outcome?: string
    outcomeUnknown?: boolean
    runtimeImpact?: string
    reconnectRequired?: boolean
    recoveryRequired?: boolean
    error?: APIError | null
  }>(operation: string, call: (requestId: string) => Promise<T>) {
    if (pending.value || stale.value || !snapshot.value || !contract.canInvoke(Domain.DomainMCP, operation)) return null
    const requestId = newMCPRequestID()
    pending.value = true
    mutationError.value = null
    lastFeedback.value = null
    try {
      let result: T
      try {
        result = await call(requestId)
      } catch {
        mutationError.value = clientError('mcp_mutation_failed', '')
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

  async function create(input: MCPConfigInput, expectedRevision: string) {
    return runMutation('create', (requestId) => desktopApi.mcpCreate(requestId, expectedRevision, input))
  }

  async function update(server: MCPManagedServer, input: MCPConfigInput, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    return runMutation('update', (requestId) =>
      desktopApi.mcpUpdate(requestId, server.name, expectedRevision, expectedGeneration, input),
    )
  }

  async function remove(server: MCPManagedServer, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    return runMutation('remove', (requestId) =>
      desktopApi.mcpRemove(requestId, server.name, expectedRevision, expectedGeneration),
    )
  }

  async function setEnabled(server: MCPManagedServer, enabled: boolean, reuseConfiguredEnvironment: boolean, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    return runMutation('setEnabled', (requestId) =>
      desktopApi.mcpSetEnabled(requestId, server.name, enabled, reuseConfiguredEnvironment, expectedRevision, expectedGeneration),
    )
  }

  async function inspect(server: MCPManagedServer) {
    if (stale.value || !snapshot.value || !contract.canInvoke(Domain.DomainMCP, 'inspect')) return null
    try {
      const result = await desktopApi.mcpInspect(server.name)
      if (result.error) {
        mutationError.value = result.error
        return result
      }
      if (
        result.registryRevision !== snapshot.value.registryRevision ||
        !result.server ||
        result.server.generation !== server.generation
      ) {
        await readSnapshot()
        return result
      }
      knownTools.value[server.name] = [...(result.tools ?? [])]
      return result
    } catch {
      mutationError.value = clientError('mcp_inspect_failed', '')
      return null
    }
  }

  async function readEnvironment(server: MCPManagedServer, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin' || stale.value || !contract.canInvoke(Domain.DomainMCP, 'environment')) return null
    try {
      const result = await desktopApi.mcpEnvironment(server.name, expectedRevision, expectedGeneration)
      if (result.error) mutationError.value = result.error
      else environments.value[server.name] = result
      return result
    } catch {
      mutationError.value = clientError('mcp_environment_failed', '')
      return null
    }
  }

  async function setEnvironment(server: MCPManagedServer, key: string, value: string, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) {
    if (server.sourceType === 'plugin') return null
    const result = await runMutation('setEnvironment', (requestId) =>
      desktopApi.mcpSetEnvironment(requestId, server.name, key, value, expectedRevision, expectedGeneration, expectedEnvRevision),
    )
    if (result && !result.error && !result.outcomeUnknown) environments.value[server.name] = result as MCPEnvironmentResult
    return result
  }

  async function unsetEnvironment(server: MCPManagedServer, key: string, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) {
    if (server.sourceType === 'plugin') return null
    const result = await runMutation('unsetEnvironment', (requestId) =>
      desktopApi.mcpUnsetEnvironment(requestId, server.name, key, expectedRevision, expectedGeneration, expectedEnvRevision),
    )
    if (result && !result.error && !result.outcomeUnknown) environments.value[server.name] = result as MCPEnvironmentResult
    return result
  }

  async function purgeEnvironment(server: MCPManagedServer, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) {
    if (server.sourceType === 'plugin') return null
    const result = await runMutation('purgeEnvironment', (requestId) =>
      desktopApi.mcpPurgeEnvironment(requestId, server.name, expectedRevision, expectedGeneration, expectedEnvRevision),
    )
    if (result && !result.error && !result.outcomeUnknown) environments.value[server.name] = result as MCPEnvironmentResult
    return result
  }

  async function reconnect(server: MCPManagedServer, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    const result = await runMutation('reconnect', (requestId) =>
      desktopApi.mcpReconnect(requestId, server.name, expectedRevision, expectedGeneration),
    )
    if (result && !result.error && !result.outcomeUnknown) knownTools.value[server.name] = [...(result.tools ?? [])]
    return result
  }

  async function readAuthorization(server: MCPManagedServer) {
    if (server.sourceType === 'plugin' || server.transport !== 'streamable_http' || !contract.canInvoke(Domain.DomainMCP, 'authorizationStatus')) return null
    try {
      const result = await desktopApi.mcpAuthorizationStatus(server.name)
      mutationError.value = result.error ?? null
      if (!result.error) {
        authorization.value = result.authorization
        callbackOptions.value = [...(result.authorization.callbackOptions ?? [])]
      }
      return result
    } catch {
      mutationError.value = clientError('mcp_authorization_status_failed', '')
      return null
    }
  }

  function stopOAuthPolling() {
    oauthPollToken += 1
    if (oauthTimer) clearTimeout(oauthTimer)
    oauthTimer = undefined
  }

  async function pollAuthorization(flowId: string, attempt = 0, token = oauthPollToken) {
    if (!flowId || token !== oauthPollToken || attempt >= maxOAuthPolls) return
    try {
      const result = await desktopApi.mcpAuthorizationFlowStatus(flowId)
      if (token !== oauthPollToken) return
      if (result.error) {
        mutationError.value = result.error
        return
      }
      authorization.value = result.authorization
      if (isTerminalOAuthStatus(result.authorization.status)) {
        stopOAuthPolling()
        await readSnapshot()
        return
      }
    } catch {
      mutationError.value = clientError('mcp_authorization_status_failed', '')
      return
    }
    oauthTimer = setTimeout(() => void pollAuthorization(flowId, attempt + 1, token), oauthPollDelayMs)
  }

  async function authorize(server: MCPManagedServer, callbackId: string, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    stopOAuthPolling()
    callbackOptions.value = []
    const result = await runMutation('authorize', (requestId) =>
      desktopApi.mcpAuthorize(requestId, server.name, callbackId, expectedRevision, expectedGeneration),
    )
    if (!result || result.error || result.outcomeUnknown) return result
    callbackOptions.value = [...(result.callbackOptions ?? [])]
    if (result.flowId) {
      const flowId = result.flowId
      authorization.value = {
        flowId,
        status: 'authorizing',
        expiresAt: result.expiresAt,
        errorCode: '',
        callbackOptions: [],
      }
      const token = oauthPollToken
      oauthTimer = setTimeout(() => void pollAuthorization(flowId, 0, token), oauthPollDelayMs)
    }
    return result
  }

  async function clearAuthorization(server: MCPManagedServer, expectedRevision: string, expectedGeneration: string) {
    if (server.sourceType === 'plugin') return null
    const result = await runMutation('clearAuthorization', (requestId) =>
      desktopApi.mcpClearAuthorization(requestId, server.name, expectedRevision, expectedGeneration),
    )
    if (result && !result.error && !result.outcomeUnknown) {
      stopOAuthPolling()
      authorization.value = { status: 'unauthorized', callbackOptions: [] }
      callbackOptions.value = []
    }
    return result
  }

  function environmentFor(name: string) {
    return environments.value[name] ?? null
  }

  function toolsFor(name: string) {
    return knownTools.value[name] ?? []
  }

  function clearTransient() {
    stopOAuthPolling()
    authorization.value = null
    callbackOptions.value = []
    mutationError.value = null
    lastFeedback.value = null
    lastOperation.value = null
  }

  return {
    snapshot,
    stale,
    loading,
    pending,
    busy,
    readError,
    mutationError,
    lastFeedback,
    lastOperation,
    servers,
    availableCount,
    attentionCount,
    authorizationRequiredCount,
    authorization,
    callbackOptions,
    canRead,
    canInvoke,
    refresh,
    create,
    update,
    remove,
    setEnabled,
    inspect,
    readEnvironment,
    setEnvironment,
    unsetEnvironment,
    purgeEnvironment,
    reconnect,
    readAuthorization,
    authorize,
    clearAuthorization,
    reconcileOperation,
    stopOAuthPolling,
    environmentFor,
    toolsFor,
    clearTransient,
  }
})
