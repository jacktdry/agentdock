import { JSONStream, type JSONSocket } from '@wailsio/runtime'
import * as EventService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/eventservice'
import type {
  APIError,
  EventSourceStatus,
  Preferences,
} from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/models'
import * as RuntimeService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/runtimeservice'
import * as SettingsService from '../../bindings/github.com/uvwt/agentdock/desktop/shared-poc/settingsservice'

export type { APIError, EventSourceStatus, Preferences }

export interface SyntheticEvent {
  sequence: number
  time: string
}

export interface ServiceStatus {
  running: boolean
  healthy: boolean
  startupEnabled: boolean
  nexusConnected: boolean
}

export interface RuntimeStatusResult {
  runtimeRoot: string
  status: ServiceStatus
  error?: APIError | null
}

export interface RuntimeActionResult {
  action: string
  completed: boolean
  error?: APIError | null
}

export interface PreferencesResult {
  preferences: Preferences
  error?: APIError | null
}

export interface SavePreferencesResult {
  saved: boolean
  error?: APIError | null
}

export interface EventBatch {
  events: SyntheticEvent[]
  produced: number
  delivered: number
  dropped: number
  queueDropped: number
  transportDropped: number
  queueDepth: number
  queueCapacity: number
  batchIntervalMs: number
}

export interface EventControlResult {
  status: EventSourceStatus
  error?: APIError | null
}

export type EventStreamState = 'connecting' | 'open' | 'closed' | 'error'

export interface EventStreamHandle {
  ready: Promise<void>
  close: () => void
}

function normaliseRuntimeStatus(result: Awaited<ReturnType<typeof RuntimeService.Status>>): RuntimeStatusResult {
  return {
    runtimeRoot: result.runtimeRoot,
    status: {
      running: result.status.running,
      healthy: result.status.healthy,
      startupEnabled: result.status.startup_enabled,
      nexusConnected: result.status.nexus_connected,
    },
    error: result.error,
  }
}

function normaliseBatch(batch: EventBatch): EventBatch {
  return {
    ...batch,
    events: batch.events ?? [],
  }
}

function openEventStream(
  onBatch: (batch: EventBatch) => void,
  onState: (state: EventStreamState) => void,
): EventStreamHandle {
  const stream: JSONSocket = JSONStream('poc:event-stream')
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
        reject(new Error('event stream failed to open'))
      }
    }
    stream.onclose = () => {
      onState('closed')
      if (!settled) {
        settled = true
        reject(new Error('event stream closed before opening'))
      }
    }
  })

  stream.onmessage = (event) => {
    onBatch(normaliseBatch(event.data as EventBatch))
  }

  onState('connecting')

  return {
    ready,
    close: () => stream.close(),
  }
}

export const desktopApi = {
  runtimeStatus: async () => normaliseRuntimeStatus(await RuntimeService.Status()),
  runtimeAction: (action: 'start' | 'stop' | 'restart') =>
    RuntimeService.Action(action) as Promise<RuntimeActionResult>,

  getPreferences: () => SettingsService.Get() as Promise<PreferencesResult>,
  savePreferences: (preferences: Preferences) =>
    SettingsService.Save(preferences) as Promise<SavePreferencesResult>,

  eventStatus: () => EventService.Status(),
  startEvents: (rateHz: number, batchIntervalMs: number) =>
    EventService.Start(rateHz, batchIntervalMs) as Promise<EventControlResult>,
  stopEvents: () => EventService.Stop(),

  openEventStream,
}
