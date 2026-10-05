import type { BasicSettings } from '../api/desktopApi'
export const logLevels = ['debug', 'info', 'warn', 'error'] as const
export function validBasicSettings(settings: BasicSettings): boolean {
  return Number.isInteger(settings.port) && settings.port >= 1 && settings.port <= 65535 &&
    logLevels.some(level => level === settings.logLevel)
}
export function settingsPayload(
  draft: BasicSettings,
  current: BasicSettings,
  autostartMutable: boolean,
  portMutable = true,
): BasicSettings {
  return {
    ...draft,
    port: portMutable ? draft.port : current.port,
    coreAutostart: autostartMutable ? draft.coreAutostart : current.coreAutostart,
  }
}
