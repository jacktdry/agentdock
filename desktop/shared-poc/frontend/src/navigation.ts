import type { MessageKey } from './i18n'

export type Section = 'overview' | 'runtime' | 'execution' | 'connection' | 'acp' | 'mcp' | 'plugin' | 'permission' | 'settings' | 'system' | 'developer'

export const primarySections: Section[] = ['overview', 'runtime', 'execution', 'connection', 'acp', 'mcp', 'plugin', 'permission', 'settings', 'system']

export const sectionKeys: Record<Section, MessageKey> = {
  overview: 'nav.overview',
  runtime: 'runtime.title',
  execution: 'execution.title',
  connection: 'connection.title',
  acp: 'acp.title',
  mcp: 'mcp.title',
  plugin: 'plugin.title',
  permission: 'permission.title',
  settings: 'basicsettings.title',
  system: 'nav.system',
  developer: 'nav.developer',
}
