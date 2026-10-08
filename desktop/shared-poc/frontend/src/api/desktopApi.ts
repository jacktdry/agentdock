import * as BrowserService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/browserservice'
import * as ConnectionService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/connectionservice'
import * as NexusService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/nexusservice'
import * as BasicSettingsService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/basicsettingsservice'
import * as UpdateService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/updateservice'
import * as DiagnosticsService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/diagnosticsservice'
import * as MCPService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/mcpservice'
import * as PluginService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/pluginservice'
import { JSONStream, type JSONSocket } from '@wailsio/runtime'
import * as ActivityProbeService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/activityprobeservice'
import * as CoreActivityService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/coreactivityservice'
import type {
  ActivityProbeStatus,
  Preferences,
  PreferencesResult,
  SavePreferencesResult,
} from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/models'
import * as SettingsService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/settingsservice'
import * as ContractService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/contractservice'
import {
  AccessLevel,
  Availability,
  Domain,
  ErrorCategory,
  RuntimeAction,
  NextDirectoryKind,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type {
  APIError,
  NexusCapabilities,
  NexusSnapshotResult,
  NexusPairRequest,
  NexusMutationResult,
  BasicSettings,
  ConnectionActionResult,
  ConnectionAutostartRequest,
  ConnectionPortRequest,
  ConnectionPreflightResult,
  ConnectionSnapshot,
  ConnectionSnapshotResult,
  ConnectionStatus,
  ConnectionTunnelRequest,
  DiagnosticsSnapshot,
  DiagnosticsDirectories,
  OAuthPasswordRevealResult,
  OperationCapability,
  PublicEndpointResult,
  UpdateStatus,
  DomainCapability,
  Manifest,
  NegotiationResult,
  RuntimeActionResult,
  RuntimeStatus,
  RuntimeStatusResult,
  MCPActionResult,
  MCPAuthCallback,
  MCPAuthorizationResult,
  MCPAuthorizationStatus,
  MCPAuthorizationStatusResult,
  MCPConfigInput,
  MCPEnvironmentResult,
  MCPManagedServer,
  MCPMutationResult,
  MCPOperationStatusResult,
  MCPReconnectResult,
  MCPServerResult,
  MCPSnapshot,
  MCPSnapshotResult,
  MCPToolSummary,
  PluginCandidate,
  PluginCandidateActionResult,
  PluginCandidateMutationInput,
  PluginCandidatePickerInput,
  PluginCandidateResult,
  PluginCandidateReview,
  PluginDetail,
  PluginEnvironmentInput,
  PluginManagedItem,
  PluginManagerSnapshot,
  PluginMCPComponent,
  PluginMutationInput,
  PluginOperationResult,
  PluginRecoveryItem,
  PluginSnapshotResult,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import {
  OAuthPasswordState,
  PortState,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopruntime/models'
import type {
  PortObservation,
  TunnelObservation,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopruntime/models'
import * as RuntimeService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/runtimeservice'
import { parseActivityBatch, type ActivityBatch, type ActivityEnvelope } from './activityContract'
import {
  parseExecutionInsertion,
  parseExecutionStreamEnvelope,
  type ExecutionStreamEnvelope,
  type ExecutionCall,
  type ExecutionEvent,
  type ExecutionInsertion,
  type ExecutionSnapshot,
  type ExecutionPage,
  type ExecutionStatus,
  type InsertionStatus,
} from './executionContract'

export {
  AccessLevel,
  Availability,
  Domain,
  ErrorCategory,
  OAuthPasswordState,
  PortState,
}
export type {
  NexusCapabilities,
  NexusSnapshotResult,
  NexusPairRequest,
  NexusMutationResult,
  ActivityBatch,
  ActivityEnvelope,
  ActivityProbeStatus,
  ExecutionCall,
  ExecutionEvent,
  ExecutionInsertion,
  ExecutionPage,
  ExecutionSnapshot,
  ExecutionStatus,
  InsertionStatus,
  APIError,
  BasicSettings,
  ConnectionActionResult,
  ConnectionAutostartRequest,
  ConnectionPortRequest,
  ConnectionPreflightResult,
  ConnectionSnapshot,
  ConnectionSnapshotResult,
  ConnectionStatus,
  ConnectionTunnelRequest,
  DiagnosticsSnapshot,
  DiagnosticsDirectories,
  OAuthPasswordRevealResult,
  OperationCapability,
  PortObservation,
  PublicEndpointResult,
  TunnelObservation,
  UpdateStatus,
  DomainCapability,
  Manifest,
  NegotiationResult,
  Preferences,
  PreferencesResult,
  RuntimeActionResult,
  RuntimeStatus,
  RuntimeStatusResult,
  SavePreferencesResult,
  MCPActionResult,
  MCPAuthCallback,
  MCPAuthorizationResult,
  MCPAuthorizationStatus,
  MCPAuthorizationStatusResult,
  MCPConfigInput,
  MCPEnvironmentResult,
  MCPManagedServer,
  MCPMutationResult,
  MCPOperationStatusResult,
  MCPReconnectResult,
  MCPServerResult,
  MCPSnapshot,
  MCPSnapshotResult,
  MCPToolSummary,
  PluginCandidate,
  PluginCandidateActionResult,
  PluginCandidateMutationInput,
  PluginCandidatePickerInput,
  PluginCandidateResult,
  PluginCandidateReview,
  PluginDetail,
  PluginEnvironmentInput,
  PluginManagedItem,
  PluginManagerSnapshot,
  PluginMCPComponent,
  PluginMutationInput,
  PluginOperationResult,
  PluginRecoveryItem,
  PluginSnapshotResult,
}

export const DESKTOP_API_VERSION = 1

export type ConnectionActionName = 'start' | 'stop' | 'restart' | 'regenerate'
export type RuntimeActionName = 'start' | 'stop' | 'restart'
export type ActivityStreamState = 'connecting' | 'open' | 'closed' | 'error'
export type ExecutionStreamState = ActivityStreamState

export interface ActivityStreamHandle {
  ready: Promise<void>
  close: () => void
}

export interface ExecutionStreamHandle {
  ready: Promise<void>
  close: () => void
}

export interface ExecutionInsertionControlResult {
  insertion: ExecutionInsertion | null
  ack: boolean
  error: APIError | null
}

const runtimeActions: Record<RuntimeActionName, RuntimeAction> = {
  start: RuntimeAction.RuntimeActionStart,
  stop: RuntimeAction.RuntimeActionStop,
  restart: RuntimeAction.RuntimeActionRestart,
}

export function clientError(code: string, caught: unknown): APIError {
  return {
    code,
    message: caught instanceof Error ? caught.message : String(caught),
    category: ErrorCategory.ErrorCategoryInternal,
    retryable: false,
  }
}

function openActivityStream(
  onBatch: (batch: ActivityBatch) => void,
  onState: (state: ActivityStreamState) => void,
  onContractError: (error: APIError) => void,
): ActivityStreamHandle {
  const stream: JSONSocket = JSONStream('desktop:activity')
  let settled = false

  const ready = new Promise<void>((resolve, reject) => {
    stream.onopen = () => {
      settled = true
      onState('open')
      resolve()
    }
    stream.onerror = () => {
      onState('error')
      if (!settled) {
        settled = true
        reject(new Error('activity stream failed to open'))
      }
    }
    stream.onclose = () => {
      onState('closed')
      if (!settled) {
        settled = true
        reject(new Error('activity stream closed before opening'))
      }
    }
  })

  stream.onmessage = (event) => {
    try {
      onBatch(parseActivityBatch(event.data))
    } catch (caught) {
      onContractError(clientError('activity_contract_invalid', caught))
    }
  }

  onState('connecting')

  return {
    ready,
    close: () => stream.close(),
  }
}

function openExecutionStream(
  onMessage: (message: ExecutionStreamEnvelope) => void,
  onState: (state: ExecutionStreamState) => void,
  onContractError: (error: APIError) => void,
): ExecutionStreamHandle {
  const stream: JSONSocket = JSONStream('desktop:execution')
  let settled = false

  const ready = new Promise<void>((resolve, reject) => {
    stream.onopen = () => {
      settled = true
      onState('open')
      resolve()
    }
    stream.onerror = () => {
      onState('error')
      if (!settled) {
        settled = true
        reject(new Error('execution stream failed to open'))
      }
    }
    stream.onclose = () => {
      onState('closed')
      if (!settled) {
        settled = true
        reject(new Error('execution stream closed before opening'))
      }
    }
  })

  stream.onmessage = (event) => {
    try {
      onMessage(parseExecutionStreamEnvelope(event.data))
    } catch (caught) {
      onContractError(clientError('execution_contract_invalid', caught))
    }
  }

  onState('connecting')

  return {
    ready,
    close: () => stream.close(),
  }
}

// Transport failures are ambiguous. Never render exceptions or retry mutations.
async function nexusCall<T>(call: () => Promise<T> & { cancel?: () => void }, timeoutMs: number): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const pending = call()
  try {
    return await Promise.race([
      pending,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => {
          reject(new Error('nexus_outcome_unknown'))
          pending.cancel?.()
        }, timeoutMs)
      }),
    ])
  } finally {
    clearTimeout(timer)
  }
}

async function nexusMutation(call: () => Promise<NexusMutationResult> & { cancel?: () => void }): Promise<NexusMutationResult> {
  try {
    return await nexusCall(call, 60_000)
  } catch {
    return {
      operationId: '', completed: false, identitySaved: false, restartRequired: false,
      error: { code: 'nexus_outcome_unknown', message: '', category: ErrorCategory.ErrorCategoryUnavailable, retryable: false },
    }
  }
}

export const desktopApi = {
  browserSnapshot: () => BrowserService.Snapshot(),
  nexusSnapshot: () => nexusCall(() => NexusService.Snapshot(), 10_000),
  nexusPair: (request: NexusPairRequest) => nexusMutation(() => NexusService.Pair(request)),
  nexusReconcile: (generation: string) => nexusMutation(() => NexusService.Reconcile(generation)),
  connectionStatus: () => ConnectionService.Status(),
  connectionSnapshot: () => ConnectionService.Snapshot(),
  connectionPreflightPort: (candidatePort: number) => ConnectionService.PreflightPort(candidatePort),
  connectionUpdatePort: (request: ConnectionPortRequest) => ConnectionService.UpdatePort(request),
  connectionConfigureTunnel: (request: ConnectionTunnelRequest) => ConnectionService.ConfigureTunnel(request),
  connectionSetTunnelAutostart: (request: ConnectionAutostartRequest) => ConnectionService.SetTunnelAutostart(request),
  connectionRevealOAuthPassword: () => ConnectionService.RevealOAuthPassword(),
  connectionTestPublicEndpoint: () => ConnectionService.TestPublicEndpoint(),
  connectionAction: (action: ConnectionActionName, configRevision: string) => ConnectionService.Action(action, configRevision),
  readBasicSettings: () => BasicSettingsService.Read(),
  saveBasicSettings: (settings: BasicSettings) => BasicSettingsService.Save(settings),
  checkUpdate: () => UpdateService.Check(),
  diagnostics: () => DiagnosticsService.Snapshot(),
  diagnosticsDirectories: () => DiagnosticsService.Directories(),
  openNextDirectory: (kind: 'logs' | 'configuration') => DiagnosticsService.OpenNextDirectory(kind as NextDirectoryKind),
  mcpSnapshot: () => MCPService.Snapshot(),
  mcpInspect: (name: string) => MCPService.Inspect(name),
  mcpCreate: (requestID: string, expectedRevision: string, input: MCPConfigInput) =>
    MCPService.Create(requestID, expectedRevision, input),
  mcpUpdate: (requestID: string, name: string, expectedRevision: string, expectedGeneration: string, input: MCPConfigInput) =>
    MCPService.Update(requestID, name, expectedRevision, expectedGeneration, input),
  mcpRemove: (requestID: string, name: string, expectedRevision: string, expectedGeneration: string) =>
    MCPService.Remove(requestID, name, expectedRevision, expectedGeneration),
  mcpSetEnabled: (requestID: string, name: string, enabled: boolean, reuseConfiguredEnvironment: boolean, expectedRevision: string, expectedGeneration: string) =>
    MCPService.SetEnabled(requestID, name, enabled, reuseConfiguredEnvironment, expectedRevision, expectedGeneration),
  mcpEnvironment: (name: string, expectedRevision: string, expectedGeneration: string) =>
    MCPService.Environment(name, expectedRevision, expectedGeneration),
  mcpSetEnvironment: (requestID: string, name: string, key: string, value: string, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) =>
    MCPService.SetEnvironment(requestID, name, key, value, expectedRevision, expectedGeneration, expectedEnvRevision),
  mcpUnsetEnvironment: (requestID: string, name: string, key: string, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) =>
    MCPService.UnsetEnvironment(requestID, name, key, expectedRevision, expectedGeneration, expectedEnvRevision),
  mcpPurgeEnvironment: (requestID: string, name: string, expectedRevision: string, expectedGeneration: string, expectedEnvRevision: string) =>
    MCPService.PurgeEnvironment(requestID, name, expectedRevision, expectedGeneration, expectedEnvRevision),
  mcpReconnect: (requestID: string, name: string, expectedRevision: string, expectedGeneration: string) =>
    MCPService.Reconnect(requestID, name, expectedRevision, expectedGeneration),
  mcpAuthorizationStatus: (name: string) => MCPService.AuthorizationStatus(name),
  mcpAuthorizationFlowStatus: (flowID: string) => MCPService.AuthorizationFlowStatus(flowID),
  mcpAuthorize: (requestID: string, name: string, callbackID: string, expectedRevision: string, expectedGeneration: string) =>
    MCPService.Authorize(requestID, name, callbackID, expectedRevision, expectedGeneration),
  mcpClearAuthorization: (requestID: string, name: string, expectedRevision: string, expectedGeneration: string) =>
    MCPService.ClearAuthorization(requestID, name, expectedRevision, expectedGeneration),
  mcpOperationStatus: (requestID: string) => MCPService.OperationStatus(requestID),
  pluginSnapshot: () => PluginService.Snapshot(),
  pluginInspect: (name: string) => PluginService.Inspect(name),
  pluginChooseCandidate: (input: PluginCandidatePickerInput) => PluginService.ChooseCandidate(input),
  pluginDiscardCandidate: (candidateID: string) => PluginService.DiscardCandidate(candidateID),
  pluginInstallCandidate: (input: PluginCandidateMutationInput) => PluginService.InstallCandidate(input),
  pluginUpdateCandidate: (input: PluginCandidateMutationInput) => PluginService.UpdateCandidate(input),
  pluginSetEnabled: (input: PluginMutationInput, enabled: boolean) => PluginService.SetEnabled(input, enabled),
  pluginRemoveKeep: (input: PluginMutationInput) => PluginService.RemoveKeep(input),
  pluginRemovePurge: (input: PluginMutationInput) => PluginService.RemovePurge(input),
  pluginEnvironment: (name: string, component: string) => PluginService.Environment(name, component),
  pluginSetEnvironment: (input: PluginEnvironmentInput) => PluginService.SetEnvironment(input),
  pluginUnsetEnvironment: (input: PluginEnvironmentInput) => PluginService.UnsetEnvironment(input),
  pluginOperationStatus: (requestID: string) => PluginService.OperationStatus(requestID),
  manifest: () => ContractService.Manifest(),
  negotiate: (domains: Domain[] = []) =>
    ContractService.Negotiate({
      clientVersion: DESKTOP_API_VERSION,
      domains,
    }),

  runtimeStatus: () => RuntimeService.Status(),
  runtimeAction: (action: RuntimeActionName) =>
    RuntimeService.Action(runtimeActions[action]),

  getPreferences: () => SettingsService.Get(),
  savePreferences: (preferences: Preferences) =>
    SettingsService.Save(preferences),

  activityStatus: () => ActivityProbeService.Status(),
  startActivityProbe: (rateHz: number, batchIntervalMs: number) =>
    ActivityProbeService.Start(rateHz, batchIntervalMs),
  stopActivityProbe: () => ActivityProbeService.Stop(),

  enqueueInsertion: async (callID: string, text: string): Promise<ExecutionInsertionControlResult> => {
    const result = await CoreActivityService.EnqueueInsertion(callID, text)
    return {
      insertion: result.insertion?.insertion_id ? parseExecutionInsertion(result.insertion) : null,
      ack: result.ack,
      error: result.error ?? null,
    }
  },
  cancelInsertion: async (insertionID: string): Promise<ExecutionInsertionControlResult> => {
    const result = await CoreActivityService.CancelInsertion(insertionID)
    return {
      insertion: result.insertion?.insertion_id ? parseExecutionInsertion(result.insertion) : null,
      ack: false,
      error: result.error ?? null,
    }
  },

  openActivityStream,
  openExecutionStream,
}
