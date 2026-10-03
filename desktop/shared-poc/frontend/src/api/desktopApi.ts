import * as ConnectionService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/connectionservice'
import * as BasicSettingsService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/basicsettingsservice'
import * as UpdateService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/updateservice'
import * as DiagnosticsService from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/diagnosticsservice'
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
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type {
  APIError,
  BasicSettings,
  ConnectionStatus,
  DiagnosticsSnapshot,
  UpdateStatus,
  DomainCapability,
  Manifest,
  NegotiationResult,
  RuntimeActionResult,
  RuntimeStatus,
  RuntimeStatusResult,
} from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
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
}
export type {
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
  ConnectionStatus,
  DiagnosticsSnapshot,
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

export const desktopApi = {
  connectionStatus: () => ConnectionService.Status(),
  connectionAction: (action: ConnectionActionName) => ConnectionService.Action(action),
  readBasicSettings: () => BasicSettingsService.Read(),
  saveBasicSettings: (settings: BasicSettings) => BasicSettingsService.Save(settings),
  checkUpdate: () => UpdateService.Check(),
  diagnostics: () => DiagnosticsService.Snapshot(),
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
