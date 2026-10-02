import { describe, expect, it } from 'vitest'
import { appendBoundedHistory, EVENT_HISTORY_LIMIT } from './boundedHistory'

const sample = (start: number, count: number) =>
  Array.from({ length: count }, (_, offset) => ({
    sequence: start + offset,
    time: `t-${start + offset}`,
  }))

describe('appendBoundedHistory', () => {
  it('retains only the newest bounded event history', () => {
    const current = sample(1, EVENT_HISTORY_LIMIT)
    const result = appendBoundedHistory(current, sample(201, 25))
    expect(result).toHaveLength(EVENT_HISTORY_LIMIT)
    expect(result[0]?.sequence).toBe(26)
    expect(result.at(-1)?.sequence).toBe(225)
  })

  it('supports a zero-size history', () => {
    expect(appendBoundedHistory(sample(1, 2), sample(3, 2), 0)).toEqual([])
  })
})
