import { readFile } from 'node:fs/promises'
import { describe, expect, it } from 'vitest'
import { fileURLToPath } from 'node:url'
import { parseActivityBatch } from './activityContract'

const fixtureUrl = new URL(
  '../../../../../internal/desktopapi/testdata/activity_batch_v1.json',
  import.meta.url,
)

describe('activity contract', () => {
  it('parses the shared Go fixture without losing uint64 cursor precision', async () => {
    const raw = await readFile(fileURLToPath(fixtureUrl), 'utf8')
    const batch = parseActivityBatch(JSON.parse(raw) as unknown)

    expect(batch.firstSequence).toBe('9007199254740993')
    expect(batch.lastSequence).toBe('9007199254740994')
    expect(batch.events[0]?.sequence).toBe('9007199254740993')
    expect(batch.publishedTotal).toBe('9007199254740994')
    expect(batch.deliveredTotal).toBe('9007199254740994')
    expect(BigInt(batch.events[0]!.sequence)).toBe(9007199254740993n)
  })

  it('rejects numeric cursors and inconsistent batch bounds', () => {
    expect(() =>
      parseActivityBatch({
        schemaVersion: 1,
        epoch: 'epoch',
        firstSequence: 1,
        lastSequence: '1',
        events: [
          {
            schemaVersion: 1,
            epoch: 'epoch',
            sequence: '1',
            occurredAt: '2026-10-03T00:00:00Z',
            kind: 'probe.tick',
            source: 'test',
          },
        ],
        publishedTotal: '1',
        deliveredTotal: '1',
        sourceDroppedTotal: '0',
        transportDroppedTotal: '0',
        queueDepth: 0,
        queueCapacity: 1,
      }),
    ).toThrow(/decimal string|string/)

    expect(() =>
      parseActivityBatch({
        schemaVersion: 1,
        epoch: 'epoch',
        firstSequence: '1',
        lastSequence: '3',
        events: [
          {
            schemaVersion: 1,
            epoch: 'epoch',
            sequence: '2',
            occurredAt: '2026-10-03T00:00:00Z',
            kind: 'probe.tick',
            source: 'test',
          },
        ],
        publishedTotal: '1',
        deliveredTotal: '1',
        sourceDroppedTotal: '0',
        transportDroppedTotal: '0',
        queueDepth: 0,
        queueCapacity: 1,
      }),
    ).toThrow(/cursor bounds/)
  })
})
