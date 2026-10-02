import type { ActivityEnvelope } from '../api/activityContract'

export const ACTIVITY_HISTORY_LIMIT = 200

export function appendBoundedHistory(
  current: ActivityEnvelope[],
  incoming: ActivityEnvelope[],
  limit = ACTIVITY_HISTORY_LIMIT,
) {
  if (limit <= 0) return []
  const combined = current.concat(incoming)
  return combined.length > limit ? combined.slice(combined.length - limit) : combined
}
