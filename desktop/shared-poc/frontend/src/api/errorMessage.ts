import type { APIError } from './desktopApi'
import { t, type MessageKey } from '../i18n'

export function errorMessage(error: APIError): string {
  const keys: Record<string, MessageKey> = {
    validation: 'errors.validation', unavailable: 'errors.unavailable',
    timeout: 'errors.timeout', compatibility: 'errors.compatibility',
  }
  // Never render platform messages or untrusted diagnostic details.
  const code = /^[a-z0-9_]{1,80}$/.test(error.code) ? error.code : 'desktop_api_error'
  return `${t(keys[error.category] ?? 'errors.operation')} (${code})`
}
