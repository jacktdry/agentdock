import type { ConnectionActionName, ConnectionStatus } from '../api/desktopApi'
export function canPerformConnection(action: ConnectionActionName, status: ConnectionStatus | null): boolean {
  return status !== null && (action !== 'regenerate' || status.mode === 'quick')
}
export function safePublicURL(raw?: string): string {
  if (!raw) return ''
  try {
    const url = new URL(raw)
    return url.protocol === 'https:' && !url.username && !url.password && !url.search &&
      !url.hash && url.pathname === '/' ? url.origin : ''
  } catch { return '' }
}
