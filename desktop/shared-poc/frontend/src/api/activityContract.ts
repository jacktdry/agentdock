export const ACTIVITY_SCHEMA_VERSION = 1

export interface ActivityEnvelope {
  schemaVersion: number
  epoch: string
  sequence: string
  occurredAt: string
  kind: string
  source: string
  data?: unknown
}

export interface ActivityBatch {
  schemaVersion: number
  epoch: string
  firstSequence: string
  lastSequence: string
  events: ActivityEnvelope[]
  publishedTotal: string
  deliveredTotal: string
  sourceDroppedTotal: string
  transportDroppedTotal: string
  queueDepth: number
  queueCapacity: number
}

const decimalCursor = /^(0|[1-9][0-9]*)$/
const utcTimestamp = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/

function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(label + ' must be an object')
  }
  return value as Record<string, unknown>
}

function asString(record: Record<string, unknown>, key: string): string {
  const value = record[key]
  if (typeof value !== 'string') throw new Error(key + ' must be a string')
  return value
}

function asCursor(record: Record<string, unknown>, key: string): string {
  const value = asString(record, key)
  if (!decimalCursor.test(value)) throw new Error(key + ' must be a decimal string')
  return value
}

function asCounter(record: Record<string, unknown>, key: string): number {
  const value = record[key]
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) {
    throw new Error(key + ' must be a non-negative safe integer')
  }
  return value
}

function parseEnvelope(value: unknown, expectedEpoch: string): ActivityEnvelope {
  const record = asRecord(value, 'activity event')
  const schemaVersion = asCounter(record, 'schemaVersion')
  if (schemaVersion !== ACTIVITY_SCHEMA_VERSION) {
    throw new Error('unsupported activity event schema version')
  }

  const epoch = asString(record, 'epoch')
  if (!epoch || epoch !== expectedEpoch) throw new Error('activity event epoch mismatch')

  const sequence = asCursor(record, 'sequence')
  const occurredAt = asString(record, 'occurredAt')
  if (!utcTimestamp.test(occurredAt) || Number.isNaN(Date.parse(occurredAt))) {
    throw new Error('occurredAt must be an RFC3339 UTC timestamp')
  }

  const kind = asString(record, 'kind')
  const source = asString(record, 'source')
  if (!kind || !source) throw new Error('activity event kind/source must be non-empty')

  const event: ActivityEnvelope = {
    schemaVersion,
    epoch,
    sequence,
    occurredAt,
    kind,
    source,
  }
  if ('data' in record) event.data = record.data
  return event
}

export function parseActivityBatch(value: unknown): ActivityBatch {
  const record = asRecord(value, 'activity batch')
  const schemaVersion = asCounter(record, 'schemaVersion')
  if (schemaVersion !== ACTIVITY_SCHEMA_VERSION) {
    throw new Error('unsupported activity batch schema version')
  }

  const epoch = asString(record, 'epoch')
  if (!epoch) throw new Error('activity epoch must be non-empty')

  const firstSequence = asCursor(record, 'firstSequence')
  const lastSequence = asCursor(record, 'lastSequence')
  const rawEvents = record.events
  if (!Array.isArray(rawEvents) || rawEvents.length === 0) {
    throw new Error('activity batch must contain at least one event')
  }

  const events = rawEvents.map((event) => parseEnvelope(event, epoch))
  if (events[0]?.sequence !== firstSequence || events.at(-1)?.sequence !== lastSequence) {
    throw new Error('activity batch cursor bounds do not match events')
  }
  for (let index = 1; index < events.length; index += 1) {
    if (BigInt(events[index - 1]!.sequence) >= BigInt(events[index]!.sequence)) {
      throw new Error('activity event sequences must be strictly increasing')
    }
  }

  return {
    schemaVersion,
    epoch,
    firstSequence,
    lastSequence,
    events,
    publishedTotal: asCursor(record, 'publishedTotal'),
    deliveredTotal: asCursor(record, 'deliveredTotal'),
    sourceDroppedTotal: asCursor(record, 'sourceDroppedTotal'),
    transportDroppedTotal: asCursor(record, 'transportDroppedTotal'),
    queueDepth: asCounter(record, 'queueDepth'),
    queueCapacity: asCounter(record, 'queueCapacity'),
  }
}
