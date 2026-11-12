import { describe, expect, it } from 'vitest'
import type { MCPConfigInput, MCPManagedServer } from '../api/desktopApi'
import {
  formatBindingLines,
  isTerminalOAuthStatus,
  mcpDisplayState,
  newMCPRequestID,
  parseBindingLines,
  validateMCPConfig,
} from './mcpLogic'

function server(partial: Partial<MCPManagedServer> = {}): MCPManagedServer {
  return {
    name: 'demo',
    displayName: 'demo',
    description: 'Demo',
    sourceType: 'standalone',
    pluginName: '',
    transport: 'stdio',
    protocolVersion: '',
    url: '',
    urlProtected: false,
    command: 'demo',
    args: [],
    argsProtected: false,
    cwd: '',
    enabled: true,
    timeoutMs: 30000,
    headerEnv: {},
    envFromEnv: {},
    generation: 'gen',
    environmentConfigured: false,
    enableRequiresEnvironmentConfirmation: false,
    blockedReasons: [],
    observation: {
      connection: 'not_connected',
      status: 'idle',
      authStatus: 'not_applicable',
      toolCount: null,
      observedAt: '',
      lastErrorCode: '',
      stale: false,
    },
    ...partial,
  }
}

describe('mcp_editor_validation', () => {
  it('accepts loopback HTTP and requires HTTPS remotely', () => {
    const local: MCPConfigInput = {
      name: 'local',
      description: 'Local',
      transport: 'streamable_http',
      url: 'http://127.0.0.1:3000/mcp',
    }
    expect(validateMCPConfig(local, true)).toEqual({})

    const remote = { ...local, url: 'http://example.com/mcp' }
    expect(validateMCPConfig(remote, true).url).toBe('mcp.validation_url_https')
  })

  it('rejects query/userinfo/fragment and secret-looking process args', () => {
    const remote: MCPConfigInput = {
      name: 'remote',
      description: 'remote_description',
      transport: 'streamable_http',
      url: 'https://user@example.com/mcp?token=x#frag',
    }
    expect(validateMCPConfig(remote, true).url).toBe('mcp.validation_url_sensitive')

    const process: MCPConfigInput = {
      name: 'local',
      description: 'Local',
      transport: 'stdio',
      command: '/usr/bin/demo',
      args: ['--api-key=secret'],
      cwd: 'relative/path',
    }
    const errors = validateMCPConfig(process, true)
    expect(errors.args).toBe('mcp.validation_args_secret')
    expect(errors.cwd).toBe('mcp.validation_cwd')
  })
})

describe('mcp_presentation_helpers', () => {
  it('keeps unknown/unavailable distinct from empty and ready', () => {
    expect(mcpDisplayState(server({ enabled: false }))).toBe('disabled')
    expect(mcpDisplayState(server({ observation: { ...server().observation, status: 'ready' } }))).toBe('available')
    expect(mcpDisplayState(server({ observation: { ...server().observation, stale: true } }))).toBe('status_unavailable')
    expect(mcpDisplayState(server({ observation: { ...server().observation, authStatus: 'auth_required' } }))).toBe('sign_in_required')
  })

  it('recognizes only terminal OAuth states', () => {
    expect(isTerminalOAuthStatus('authorized')).toBe(true)
    expect(isTerminalOAuthStatus('denied')).toBe(true)
    expect(isTerminalOAuthStatus('authorizing')).toBe(false)
  })

  it('creates exact 32-character request ids', () => {
    const fake = {
      getRandomValues(array: Uint8Array) {
        array.forEach((_, index) => { array[index] = index })
        return array
      },
    }
    expect(newMCPRequestID(fake as Pick<Crypto, 'getRandomValues'>)).toBe('000102030405060708090a0b0c0d0e0f')
  })

  it('round-trips binding text without values leaking elsewhere', () => {
    const parsed = parseBindingLines('Authorization=REMOTE_TOKEN\nCHILD=HOST_VALUE\n')
    expect(parsed).toEqual({ Authorization: 'REMOTE_TOKEN', CHILD: 'HOST_VALUE' })
    expect(formatBindingLines(parsed)).toBe('Authorization=REMOTE_TOKEN\nCHILD=HOST_VALUE')
  })
})
