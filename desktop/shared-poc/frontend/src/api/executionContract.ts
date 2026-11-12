export const EXECUTION_SCHEMA_VERSION = 1
export const MAX_EXECUTION_EVENTS = 200
export const MAX_VISIBLE_CALLS = 120

export type ExecutionStatus = 'running' | 'waiting_for_user' | 'completed' | 'failed' | 'cancelled'
export type InsertionStatus = 'accepted' | 'delivered' | 'rejected' | 'expired' | 'cancelled'

export interface ExecutionCall {
  callID: string
  parentCallID: string
  tool: string
  source: string
  status: ExecutionStatus
  insertionSupported: boolean
  startedSequence: string
  endedSequence: string
  startedAt: string
  updatedAt: string
  completedAt: string
  errorCode: string
  errorCategory: string
  continuationID: string
}

export interface ExecutionInsertion {
  insertionID: string
  targetCallID: string
  status: InsertionStatus
  textPreview: string
  textBytes: number
  createdAt: string
  updatedAt: string
  expiresAt: string
  deliveredAt: string
  reason: string
}

export interface OutputFact {
  continuationID: string
  status: string
  exitCode: number | null
  commandOK: boolean | null
  timedOut: boolean
  stdoutTotalBytes: number
  stderrTotalBytes: number
  stdoutTruncated: boolean
  stderrTruncated: boolean
}

export interface FileChangeFact {
  action: string
  paths: string[]
  filesChanged: number
  insertions: number
  deletions: number
  statsKnown: boolean
  truncated: boolean
}

export interface ExecutionEvent {
  schemaVersion: number
  epoch: string
  sequence: string
  occurredAt: string
  kind: string
  callID: string
  parentCallID: string
  tool: string
  source: string
  status: ExecutionStatus | ''
  insertionSupported: boolean
  errorCode: string
  errorCategory: string
  insertionID: string
  output: OutputFact | null
  fileChange: FileChangeFact | null
}

export interface ExecutionSnapshot {
  schemaVersion: number
  epoch: string
  latestSequence: string
  prunedThrough: string
  activeCalls: number
  calls: ExecutionCall[]
  insertions: ExecutionInsertion[]
}

export interface ExecutionPage {
  schemaVersion: number
  epoch: string
  after: string
  latestSequence: string
  prunedThrough: string
  gap: boolean
  hasMore: boolean
  events: ExecutionEvent[]
}

export interface ExecutionStreamEnvelope {
  kind: 'snapshot' | 'activity' | 'status' | 'error'
  schemaVersion: number
  snapshot: ExecutionSnapshot | null
  page: ExecutionPage | null
  id: string
  code: string
  reconnectTotal: string
  transportDroppedTotal: string
}

const executionStatuses = new Set<ExecutionStatus>([
  'running',
  'waiting_for_user',
  'completed',
  'failed',
  'cancelled',
])

const insertionStatuses = new Set<InsertionStatus>([
  'accepted',
  'delivered',
  'rejected',
  'expired',
  'cancelled',
])

function record(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${name} must be an object`)
  }
  return value as Record<string, unknown>
}

function stringValue(value: unknown, name: string, optional = false): string {
  if ((value === undefined || value === null) && optional) return ''
  if (typeof value !== 'string') throw new Error(`${name} must be a string`)
  return value
}

function decimal(value: unknown, name: string): string {
  const text = stringValue(value, name)
  if (!/^(0|[1-9][0-9]*)$/.test(text)) throw new Error(`${name} must be an unsigned decimal string`)
  return text
}

function finiteNumber(value: unknown, name: string, optional = false): number {
  if ((value === undefined || value === null) && optional) return 0
  if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error(`${name} must be a finite number`)
  return value
}

function bool(value: unknown, name: string, optional = false): boolean {
  if ((value === undefined || value === null) && optional) return false
  if (typeof value !== 'boolean') throw new Error(`${name} must be a boolean`)
  return value
}

function executionStatus(value: unknown, name: string, optional = false): ExecutionStatus | '' {
  if ((value === undefined || value === null || value === '') && optional) return ''
  const text = stringValue(value, name)
  if (!executionStatuses.has(text as ExecutionStatus)) throw new Error(`${name} has an unsupported execution status`)
  return text as ExecutionStatus
}

function insertionStatus(value: unknown, name: string): InsertionStatus {
  const text = stringValue(value, name)
  if (!insertionStatuses.has(text as InsertionStatus)) throw new Error(`${name} has an unsupported insertion status`)
  return text as InsertionStatus
}

function parseCall(value: unknown, index: number): ExecutionCall {
  const item = record(value, `calls[${index}]`)
  return {
    callID: stringValue(item.call_id, `calls[${index}].call_id`),
    parentCallID: stringValue(item.parent_call_id, `calls[${index}].parent_call_id`, true),
    tool: stringValue(item.tool, `calls[${index}].tool`),
    source: stringValue(item.source, `calls[${index}].source`),
    status: executionStatus(item.status, `calls[${index}].status`) as ExecutionStatus,
    insertionSupported: bool(item.insertion_supported, `calls[${index}].insertion_supported`, true),
    startedSequence: decimal(item.started_sequence, `calls[${index}].started_sequence`),
    endedSequence: item.ended_sequence === undefined ? '' : decimal(item.ended_sequence, `calls[${index}].ended_sequence`),
    startedAt: stringValue(item.started_at, `calls[${index}].started_at`),
    updatedAt: stringValue(item.updated_at, `calls[${index}].updated_at`),
    completedAt: stringValue(item.completed_at, `calls[${index}].completed_at`, true),
    errorCode: stringValue(item.error_code, `calls[${index}].error_code`, true),
    errorCategory: stringValue(item.error_category, `calls[${index}].error_category`, true),
    continuationID: stringValue(item.continuation_id, `calls[${index}].continuation_id`, true),
  }
}

export function parseExecutionInsertion(value: unknown, index = 0): ExecutionInsertion {
  const item = record(value, `insertions[${index}]`)
  return {
    insertionID: stringValue(item.insertion_id, `insertions[${index}].insertion_id`),
    targetCallID: stringValue(item.target_call_id, `insertions[${index}].target_call_id`),
    status: insertionStatus(item.status, `insertions[${index}].status`),
    textPreview: stringValue(item.text_preview, `insertions[${index}].text_preview`, true),
    textBytes: finiteNumber(item.text_bytes, `insertions[${index}].text_bytes`),
    createdAt: stringValue(item.created_at, `insertions[${index}].created_at`),
    updatedAt: stringValue(item.updated_at, `insertions[${index}].updated_at`),
    expiresAt: stringValue(item.expires_at, `insertions[${index}].expires_at`),
    deliveredAt: stringValue(item.delivered_at, `insertions[${index}].delivered_at`, true),
    reason: stringValue(item.reason, `insertions[${index}].reason`, true),
  }
}

function parseOutput(value: unknown): OutputFact | null {
  if (value === undefined || value === null) return null
  const item = record(value, 'event.output')
  const exitCode = item.exit_code === undefined || item.exit_code === null ? null : finiteNumber(item.exit_code, 'event.output.exit_code')
  const commandOK = item.command_ok === undefined || item.command_ok === null ? null : bool(item.command_ok, 'event.output.command_ok')
  return {
    continuationID: stringValue(item.continuation_id, 'event.output.continuation_id', true),
    status: stringValue(item.status, 'event.output.status', true),
    exitCode,
    commandOK,
    timedOut: bool(item.timed_out, 'event.output.timed_out', true),
    stdoutTotalBytes: finiteNumber(item.stdout_total_bytes, 'event.output.stdout_total_bytes', true),
    stderrTotalBytes: finiteNumber(item.stderr_total_bytes, 'event.output.stderr_total_bytes', true),
    stdoutTruncated: bool(item.stdout_truncated, 'event.output.stdout_truncated', true),
    stderrTruncated: bool(item.stderr_truncated, 'event.output.stderr_truncated', true),
  }
}

function parseFileChange(value: unknown): FileChangeFact | null {
  if (value === undefined || value === null) return null
  const item = record(value, 'event.file_change')
  if (item.paths !== undefined && !Array.isArray(item.paths)) throw new Error('event.file_change.paths must be an array')
  const paths = (item.paths as unknown[] | undefined ?? []).map((path, index) =>
    stringValue(path, `event.file_change.paths[${index}]`),
  )
  return {
    action: stringValue(item.action, 'event.file_change.action', true),
    paths,
    filesChanged: finiteNumber(item.files_changed, 'event.file_change.files_changed', true),
    insertions: finiteNumber(item.insertions, 'event.file_change.insertions', true),
    deletions: finiteNumber(item.deletions, 'event.file_change.deletions', true),
    statsKnown: bool(item.stats_known, 'event.file_change.stats_known', true),
    truncated: bool(item.truncated, 'event.file_change.truncated', true),
  }
}

function parseEvent(value: unknown, index: number, epoch: string): ExecutionEvent {
  const item = record(value, `events[${index}]`)
  const eventEpoch = stringValue(item.epoch, `events[${index}].epoch`)
  if (eventEpoch !== epoch) throw new Error('execution event epoch does not match page epoch')
  return {
    schemaVersion: finiteNumber(item.schema_version, `events[${index}].schema_version`),
    epoch: eventEpoch,
    sequence: decimal(item.sequence, `events[${index}].sequence`),
    occurredAt: stringValue(item.occurred_at, `events[${index}].occurred_at`),
    kind: stringValue(item.kind, `events[${index}].kind`),
    callID: stringValue(item.call_id, `events[${index}].call_id`, true),
    parentCallID: stringValue(item.parent_call_id, `events[${index}].parent_call_id`, true),
    tool: stringValue(item.tool, `events[${index}].tool`, true),
    source: stringValue(item.source, `events[${index}].source`, true),
    status: executionStatus(item.status, `events[${index}].status`, true),
    insertionSupported: bool(item.insertion_supported, `events[${index}].insertion_supported`, true),
    errorCode: stringValue(item.error_code, `events[${index}].error_code`, true),
    errorCategory: stringValue(item.error_category, `events[${index}].error_category`, true),
    insertionID: stringValue(item.insertion_id, `events[${index}].insertion_id`, true),
    output: parseOutput(item.output),
    fileChange: parseFileChange(item.file_change),
  }
}

export function parseExecutionSnapshot(value: unknown): ExecutionSnapshot {
  const item = record(value, 'snapshot')
  const schemaVersion = finiteNumber(item.schema_version, 'snapshot.schema_version')
  if (schemaVersion !== EXECUTION_SCHEMA_VERSION) throw new Error('unsupported execution snapshot version')
  const epoch = stringValue(item.epoch, 'snapshot.epoch')
  if (!epoch) throw new Error('execution snapshot epoch is required')
  if (!Array.isArray(item.calls)) throw new Error('snapshot.calls must be an array')
  const rawInsertions = item.insertions === undefined ? [] : item.insertions
  if (!Array.isArray(rawInsertions)) throw new Error('snapshot.insertions must be an array')
  return {
    schemaVersion,
    epoch,
    latestSequence: decimal(item.latest_sequence, 'snapshot.latest_sequence'),
    prunedThrough: decimal(item.pruned_through, 'snapshot.pruned_through'),
    activeCalls: finiteNumber(item.active_calls, 'snapshot.active_calls'),
    calls: item.calls.map(parseCall),
    insertions: rawInsertions.map(parseExecutionInsertion),
  }
}

export function parseExecutionPage(value: unknown): ExecutionPage {
  const item = record(value, 'page')
  const schemaVersion = finiteNumber(item.schema_version, 'page.schema_version')
  if (schemaVersion !== EXECUTION_SCHEMA_VERSION) throw new Error('unsupported execution page version')
  const epoch = stringValue(item.epoch, 'page.epoch')
  if (!epoch) throw new Error('execution page epoch is required')
  if (!Array.isArray(item.events)) throw new Error('page.events must be an array')
  return {
    schemaVersion,
    epoch,
    after: decimal(item.after, 'page.after'),
    latestSequence: decimal(item.latest_sequence, 'page.latest_sequence'),
    prunedThrough: decimal(item.pruned_through, 'page.pruned_through'),
    gap: bool(item.gap, 'page.gap'),
    hasMore: bool(item.has_more, 'page.has_more'),
    events: item.events.map((event, index) => parseEvent(event, index, epoch)),
  }
}

export function parseExecutionStreamEnvelope(value: unknown): ExecutionStreamEnvelope {
  const item = record(value, 'execution stream envelope')
  const schemaVersion = finiteNumber(item.schemaVersion, 'envelope.schemaVersion')
  if (schemaVersion !== EXECUTION_SCHEMA_VERSION) throw new Error('unsupported execution stream version')
  const kind = stringValue(item.kind, 'envelope.kind')
  if (kind !== 'snapshot' && kind !== 'activity' && kind !== 'status' && kind !== 'error') {
    throw new Error('unsupported execution stream message kind')
  }
  return {
    kind,
    schemaVersion,
    snapshot: kind === 'snapshot' ? parseExecutionSnapshot(item.snapshot) : null,
    page: kind === 'activity' ? parseExecutionPage(item.page) : null,
    id: stringValue(item.id, 'envelope.id', true),
    code: stringValue(item.code, 'envelope.code', true),
    reconnectTotal: item.reconnectTotal === undefined ? '0' : decimal(item.reconnectTotal, 'envelope.reconnectTotal'),
    transportDroppedTotal: item.transportDroppedTotal === undefined ? '0' : decimal(item.transportDroppedTotal, 'envelope.transportDroppedTotal'),
  }
}
