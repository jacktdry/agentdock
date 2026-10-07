import type { MCPConfigInput, MCPManagedServer } from '../api/desktopApi'

export type MCPDisplayState =
  | 'not_connected'
  | 'available'
  | 'disabled'
  | 'sign_in_required'
  | 'signing_in'
  | 'needs_attention'
  | 'status_unavailable'

export interface MCPValidationErrors {
  name?: string
  description?: string
  transport?: string
  url?: string
  command?: string
  cwd?: string
  args?: string
  timeoutMs?: string
}

const serverNamePattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/
const sensitiveArgPattern = /(?:^|[-_])(token|secret|password|passwd|api[-_]?key)(?:=|:)/i

function loopbackHost(hostname: string) {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, '')
  return host === 'localhost' || host === '::1' || host.startsWith('127.')
}

function absolutePath(path: string) {
  if (!path) return true
  return path.startsWith('/') || /^[A-Za-z]:[\\/]/.test(path) || path.startsWith('\\\\')
}

export function validateMCPConfig(input: MCPConfigInput, creating = false): MCPValidationErrors {
  const errors: MCPValidationErrors = {}
  const name = (input.name ?? '').trim()
  const description = (input.description ?? '').trim()
  const transport = (input.transport ?? '').trim()

  if (creating && !serverNamePattern.test(name)) errors.name = 'mcp.validation_name'
  if (!description) errors.description = 'mcp.validation_description'
  if (transport !== 'streamable_http' && transport !== 'stdio') errors.transport = 'mcp.validation_transport'

  if (transport === 'streamable_http') {
    const raw = (input.url ?? '').trim()
    try {
      const parsed = new URL(raw)
      if (!['http:', 'https:'].includes(parsed.protocol)) {
        errors.url = 'mcp.validation_url_protocol'
      } else if (parsed.username || parsed.password || parsed.hash || parsed.search) {
        errors.url = 'mcp.validation_url_sensitive'
      } else if (!loopbackHost(parsed.hostname) && parsed.protocol !== 'https:') {
        errors.url = 'mcp.validation_url_https'
      }
    } catch {
      errors.url = 'mcp.validation_url'
    }
  }

  if (transport === 'stdio') {
    if (!(input.command ?? '').trim()) errors.command = 'mcp.validation_command'
    if (!absolutePath((input.cwd ?? '').trim())) errors.cwd = 'mcp.validation_cwd'
    if ((input.args ?? []).some((arg) => sensitiveArgPattern.test(arg.trim()))) {
      errors.args = 'mcp.validation_args_secret'
    }
  }

  if (input.timeoutMs != null && (input.timeoutMs < 1 || input.timeoutMs > 300000)) {
    errors.timeoutMs = 'mcp.validation_timeout'
  }

  return errors
}

export function mcpDisplayState(server: MCPManagedServer): MCPDisplayState {
  if (!server.enabled) return 'disabled'
  if (server.observation.authStatus === 'auth_required') return 'sign_in_required'
  if (server.observation.authStatus === 'authorizing') return 'signing_in'
  if (server.observation.stale || server.observation.status === 'unknown') return 'status_unavailable'
  if (server.observation.status === 'ready') return 'available'
  if (server.observation.status === 'error' || server.observation.lastErrorCode) return 'needs_attention'
  return 'not_connected'
}

export function isMCPAttention(server: MCPManagedServer) {
  const state = mcpDisplayState(server)
  return state === 'sign_in_required' || state === 'needs_attention' || state === 'status_unavailable'
}

export function isTerminalOAuthStatus(status: string) {
  return ['authorized', 'denied', 'expired', 'failed', 'cancelled'].includes(status)
}

export function newMCPRequestID(source: Pick<Crypto, 'getRandomValues'> = globalThis.crypto) {
  if (!source?.getRandomValues) throw new Error('secure request id generator unavailable')
  const bytes = new Uint8Array(16)
  source.getRandomValues(bytes)
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
}

export function parseBindingLines(text: string) {
  const result: Record<string, string> = {}
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line) continue
    const index = line.indexOf('=')
    if (index <= 0) continue
    const key = line.slice(0, index).trim()
    const value = line.slice(index + 1).trim()
    if (key && value) result[key] = value
  }
  return result
}

export function formatBindingLines(values: Record<string, string | undefined> | null | undefined) {
  return Object.entries(values ?? {})
    .filter((entry): entry is [string, string] => typeof entry[1] === 'string' && entry[1].length > 0)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}
