import type { PluginCandidateReview, PluginManagedItem } from '../api/desktopApi'

export function newPluginRequestID(source: Pick<Crypto, 'getRandomValues'> = globalThis.crypto) {
  if (!source?.getRandomValues) throw new Error('secure request id generator unavailable')
  const bytes = new Uint8Array(16)
  source.getRandomValues(bytes)
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
}

export function pluginNeedsAttention(plugin: PluginManagedItem) {
  return Boolean(plugin.recoveryState) || plugin.warningCount > 0
}

export function shortFingerprint(value: string) {
  const trimmed = value.trim()
  if (trimmed.length <= 24) return trimmed
  return trimmed.slice(0, 14) + '…' + trimmed.slice(-8)
}

export function candidateSummary(review: PluginCandidateReview) {
  return {
    skills: review.skills?.length ?? 0,
    mcp: review.mcp?.length ?? 0,
    executables: review.executables?.length ?? 0,
    warnings: review.warnings?.length ?? 0,
    issues: review.issues?.length ?? 0,
  }
}

export interface PluginNamedDiff {
  added: string[]
  removed: string[]
  changed: string[]
}

export interface PluginCandidateDiff {
  fingerprintChanged: boolean
  skills: PluginNamedDiff
  mcp: PluginNamedDiff
  executables: { added: string[]; removed: string[] }
  warnings: { added: string[]; resolved: string[] }
}

function namedDiff<T>(
  current: T[],
  next: T[],
  name: (item: T) => string,
  signature: (item: T) => string,
): PluginNamedDiff {
  const before = new Map(current.map((item) => [name(item), signature(item)]))
  const after = new Map(next.map((item) => [name(item), signature(item)]))
  const added = [...after.keys()].filter((key) => !before.has(key)).sort()
  const removed = [...before.keys()].filter((key) => !after.has(key)).sort()
  const changed = [...after.keys()].filter((key) => before.has(key) && before.get(key) !== after.get(key)).sort()
  return { added, removed, changed }
}

function stringSetDiff(current: string[], next: string[]) {
  const before = new Set(current)
  const after = new Set(next)
  return {
    added: [...after].filter((value) => !before.has(value)).sort(),
    removed: [...before].filter((value) => !after.has(value)).sort(),
  }
}

export function candidateDiff(current: PluginCandidateReview | null | undefined, next: PluginCandidateReview): PluginCandidateDiff | null {
  if (!current) return null
  const executableDiff = stringSetDiff(current.executables ?? [], next.executables ?? [])
  const warningDiff = stringSetDiff(current.warnings ?? [], next.warnings ?? [])
  return {
    fingerprintChanged: current.packageFingerprint !== next.packageFingerprint,
    skills: namedDiff(
      current.skills ?? [],
      next.skills ?? [],
      (item) => item.name,
      (item) => JSON.stringify([item.description]),
    ),
    mcp: namedDiff(
      current.mcp ?? [],
      next.mcp ?? [],
      (item) => item.name,
      (item) => JSON.stringify([
        item.description,
        item.transport,
        item.endpoint ?? '',
        item.command ?? '',
        [...(item.environmentNames ?? [])].sort(),
        [...(item.headerNames ?? [])].sort(),
      ]),
    ),
    executables: executableDiff,
    warnings: { added: warningDiff.added, resolved: warningDiff.removed },
  }
}
