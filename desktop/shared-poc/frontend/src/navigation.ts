import type { MessageKey } from './i18n'
export type Section = 'overview' | 'runtime' | 'connection' | 'settings' | 'system' | 'developer'
export const primarySections: Section[] = ['overview', 'runtime', 'connection', 'settings', 'system']
export const sectionKeys: Record<Section, MessageKey> = {
  overview: 'nav.overview', runtime: 'runtime.title', connection: 'connection.title',
  settings: 'basicsettings.title', system: 'nav.system', developer: 'nav.developer',
}
