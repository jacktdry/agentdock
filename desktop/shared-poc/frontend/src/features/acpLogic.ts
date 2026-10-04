import type { ACPLifecycleUpdate } from '../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'

export const lifecyclePolicies = ['persistent', 'ephemeral', 'idle-managed'] as const
export const policyKeys = { persistent: 'acp.policy_persistent', ephemeral: 'acp.policy_ephemeral', 'idle-managed': 'acp.policy_idle_managed' } as const
export type LifecyclePolicy = typeof lifecyclePolicies[number]
// Mirrored from internal/acp/lifecycle_policy.go; the backend remains authoritative.
export const idleMinutes = { min: 1, default: 30, max: 7 * 24 * 60 }

export function lifecycleUpdate(profileId: string, sessionId: string, policy: string, minutes: number): ACPLifecycleUpdate | null {
  if (!lifecyclePolicies.includes(policy as LifecyclePolicy)) return null
  if (policy !== 'idle-managed') return { profileId, sessionId, policy }
  const ms = minutes * 60_000
  if (!Number.isSafeInteger(ms) || minutes < idleMinutes.min || minutes > idleMinutes.max) return null
  return { profileId, sessionId, policy, idleCloseAfterMs: ms }
}

// Metadata only: never render adapter error strings, commands, API messages or details.
export function safeMetadata(value: string | null | undefined): string {
  if (!value || /bearer\s|(?:token|secret|password|api[_-]?key)\s*[=:]|eyJ[a-zA-Z0-9_-]+\./i.test(value)) return ''
  return value
}

export function safeEndpoint(value: string): string {
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol)) return ''
    url.username = ''
    url.password = ''
    url.search = ''
    url.hash = ''
    return safeMetadata(url.toString())
  } catch { return '' }
}
