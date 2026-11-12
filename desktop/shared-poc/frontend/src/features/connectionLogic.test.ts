import { describe, expect, it } from 'vitest'
import {
  AccessLevel,
  Availability,
  OAuthPasswordState,
  PortState,
  type ConnectionSnapshot,
} from '../api/desktopApi'
import {
  actionablePortObservation,
  canPerformConnection,
  connectorReadiness,
  namedOriginFromPublicMCPURL,
  safeMCPURL,
  validNamedOrigin,
} from './connectionLogic'

function snapshot(overrides: Partial<ConnectionSnapshot> = {}): ConnectionSnapshot {
  return {
    portObservation: {
      configuredPort: 8767,
      observedPort: 8767,
      state: PortState.PortOwnedByNext,
      reasonCode: 'selected_next_process',
      observedAt: '2026-10-05T00:00:00Z',
      configRevision: 'rev-1',
    },
    coreRunning: true,
    coreHealth: 'healthy',
    tunnel: {
      registered: true,
      running: true,
      connected: true,
      autostart: 'enabled',
      token_state: 'stored',
      tunnel_generation: 'gen-1',
      public_endpoint: 'unchecked',
      recovery_required: false,
    },
    operations: [
      {
        name: 'restart',
        access: AccessLevel.AccessMutating,
        requiresConfirmation: true,
        availability: Availability.AvailabilityAvailable,
      },
      {
        name: 'regenerate',
        access: AccessLevel.AccessMutating,
        requiresConfirmation: true,
        availability: Availability.AvailabilityUnavailable,
        disabledReason: 'quick_only',
      },
    ],
    localMCPURL: 'http://127.0.0.1:8767/mcp',
    publicMCPURL: 'https://next.example.test/mcp',
    port: 8767,
    mode: 'named',
    oauthEnabled: true,
    oauthPasswordState: OAuthPasswordState.OAuthPasswordStored,
    configRevision: 'rev-1',
    tunnelGeneration: 'gen-1',
    ...overrides,
  }
}

describe('connection logic', () => {
  it('uses backend operation capability instead of legacy mode guesses', () => {
    const value = snapshot()
    expect(canPerformConnection('restart', value)).toBe(true)
    expect(canPerformConnection('regenerate', value)).toBe(false)
    expect(canPerformConnection('missing', value)).toBe(false)
    expect(canPerformConnection('restart', null)).toBe(false)
  })

  it('accepts only connector-shaped URLs', () => {
    expect(safeMCPURL('http://127.0.0.1:8767/mcp')).toBe('http://127.0.0.1:8767/mcp')
    expect(safeMCPURL('http://localhost:8767/mcp')).toBe('http://localhost:8767/mcp')
    expect(safeMCPURL('https://example.com/mcp', true)).toBe('https://example.com/mcp')
    expect(safeMCPURL('https://user:pass@example.com/mcp', true)).toBe('')
    expect(safeMCPURL('https://example.com/other', true)).toBe('')
    expect(safeMCPURL('http://example.com/mcp')).toBe('')
  })

  it('derives and validates named HTTPS origins without paths or credentials', () => {
    expect(namedOriginFromPublicMCPURL('https://next.example.test/mcp')).toBe('https://next.example.test')
    expect(namedOriginFromPublicMCPURL('http://next.example.test/mcp')).toBe('')
    expect(validNamedOrigin('https://next.example.test')).toBe(true)
    expect(validNamedOrigin('https://next.example.test/path')).toBe(false)
    expect(validNamedOrigin('http://next.example.test')).toBe(false)
    expect(validNamedOrigin('https://user:pass@next.example.test')).toBe(false)
  })

  it('requires a free candidate before a port update', () => {
    const available = {
      ...snapshot().portObservation,
      observedPort: 19000,
      state: PortState.PortAvailable,
    }
    expect(actionablePortObservation(available, 19000, 8767)).toBe(true)
    expect(actionablePortObservation({ ...available, state: PortState.PortConflict }, 19000, 8767)).toBe(false)
    expect(actionablePortObservation({ ...available, state: PortState.PortUnknown }, 19000, 8767)).toBe(false)
    expect(actionablePortObservation(available, 19001, 8767)).toBe(false)
  })

  it('does not claim connector readiness before public reachability is proven', () => {
    const value = snapshot()
    expect(connectorReadiness(value, 'unchecked')).toEqual({ local: true, public: false })
    expect(connectorReadiness(value, 'reachable')).toEqual({ local: true, public: true })
    expect(connectorReadiness({ ...value, oauthPasswordState: OAuthPasswordState.OAuthPasswordMissing }, 'reachable'))
      .toEqual({ local: true, public: false })
    expect(connectorReadiness({ ...value, coreRunning: false }, 'reachable'))
      .toEqual({ local: false, public: false })
  })
})
