import { describe, expect, it } from 'vitest'
import type { ConnectionStatus } from '../api/desktopApi'
import { canPerformConnection, safePublicURL } from './connectionLogic'

const status = (mode: string): ConnectionStatus => ({
  mode,
  running: true,
  ready: true,
  startupEnabled: true,
  publicURL: 'https://example.com',
})

describe('connection logic', () => {
  it('allows regenerate only for quick tunnels', () => {
    expect(canPerformConnection('regenerate', status('quick'))).toBe(true)
    expect(canPerformConnection('regenerate', status('named'))).toBe(false)
    expect(canPerformConnection('regenerate', status('tailscale'))).toBe(false)
    expect(canPerformConnection('restart', status('named'))).toBe(true)
    expect(canPerformConnection('restart', null)).toBe(false)
  })

  it('keeps only secret-safe HTTPS origins', () => {
    expect(safePublicURL('https://example.com/')).toBe('https://example.com')
    expect(safePublicURL('http://example.com/')).toBe('')
    expect(safePublicURL('https://user:pass@example.com/')).toBe('')
    expect(safePublicURL('https://example.com/path')).toBe('')
    expect(safePublicURL('https://example.com/?token=secret')).toBe('')
  })
})
