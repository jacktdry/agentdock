import type { SyntheticEvent } from '../api/desktopApi'

export const EVENT_HISTORY_LIMIT = 200

export function appendBoundedHistory(
  current: SyntheticEvent[],
  incoming: SyntheticEvent[],
  limit = EVENT_HISTORY_LIMIT,
) {
  if (limit <= 0) return []
  const combined = current.concat(incoming)
  return combined.length > limit ? combined.slice(combined.length - limit) : combined
}
