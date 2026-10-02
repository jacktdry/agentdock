import { describe, expect, it } from 'vitest'
import type { ActivityEnvelope } from '../api/activityContract'
import { ACTIVITY_HISTORY_LIMIT, appendBoundedHistory } from './boundedHistory'

const sample = (start: number, count: number): ActivityEnvelope[] =>
  Array.from({ length: count }, (_, offset) => {
    const sequence = String(start + offset)
    return {
      schemaVersion: 1,
      epoch: 'test-epoch',
      sequence,
      occurredAt: '2026-10-03T00:00:00Z',
      kind: 'probe.tick',
      source: 'test',
    }
  })

describe('appendBoundedHistory', () => {
  it('retains only the newest bounded activity history', () => {
    const current = sample(1, ACTIVITY_HISTORY_LIMIT)
    const result = appendBoundedHistory(current, sample(201, 25))
    expect(result).toHaveLength(ACTIVITY_HISTORY_LIMIT)
    expect(result[0]?.sequence).toBe('26')
    expect(result.at(-1)?.sequence).toBe('225')
  })

  it('supports a zero-size history', () => {
    expect(appendBoundedHistory(sample(1, 2), sample(3, 2), 0)).toEqual([])
  })
})
