import { describe, expect, it } from 'vitest'
import { idleMinutes, lifecycleUpdate, safeEndpoint, safeMetadata } from './acpLogic'

describe('ACP lifecycle payloads', () => {
  it('maps the documented min/default/max from minutes to backend milliseconds', () => {
    for (const minutes of [idleMinutes.min, idleMinutes.default, idleMinutes.max, 1.5]) {
      expect(lifecycleUpdate('profile', 'session', 'idle-managed', minutes)).toEqual({
        profileId: 'profile', sessionId: 'session', policy: 'idle-managed', idleCloseAfterMs: minutes * 60000,
      })
    }
  })
  it('rejects unsupported policies and invalid TTL rather than silently clamping', () => {
    for (const ttl of [0, -1, 10081, NaN, Infinity, 1.000000001]) {
      expect(lifecycleUpdate('p', 's', 'idle-managed', ttl)).toBeNull()
    }
    expect(lifecycleUpdate('p', 's', 'automatic', 30)).toBeNull()
  })
  it('omits TTL for policies that forbid it', () => {
    for (const policy of ['persistent', 'ephemeral']) {
      expect(lifecycleUpdate('p', 's', policy, 30)).toEqual({ profileId: 'p', sessionId: 's', policy })
    }
  })
})
describe('ACP metadata display', () => {
  it('strips endpoint authentication, query parameters and fragments', () => {
    expect(safeEndpoint('http://user:secret@127.0.0.1:8766/mcp?token=private#private')).toBe('http://127.0.0.1:8766/mcp')
    expect(safeEndpoint('file:///secret')).toBe('')
    expect(safeEndpoint('invalid')).toBe('')
  })
  it('suppresses recognizable credential metadata', () => {
    for (const value of ['Bearer private', 'token=private', 'password: private', 'eyJabc.payload.signature']) expect(safeMetadata(value)).toBe('')
    expect(safeMetadata('sqlite-vec')).toBe('sqlite-vec')
  })
})
