import { describe, expect, it } from 'vitest'
import { settingsPayload, validBasicSettings } from './settingsLogic'

describe('basic settings logic', () => {
  it('validates the shared port and log-level contract', () => {
    expect(validBasicSettings({ port: 23110, logLevel: 'info', coreAutostart: true })).toBe(true)
    expect(validBasicSettings({ port: 0, logLevel: 'info', coreAutostart: true })).toBe(false)
    expect(validBasicSettings({ port: 65536, logLevel: 'info', coreAutostart: true })).toBe(false)
    expect(validBasicSettings({ port: 23110, logLevel: 'trace', coreAutostart: true })).toBe(false)
  })

  it('preserves native-owned autostart when the capability is immutable', () => {
    const current = { port: 23110, logLevel: 'info', coreAutostart: true }
    const draft = { port: 24110, logLevel: 'debug', coreAutostart: false }

    expect(settingsPayload(draft, current, false)).toEqual({
      port: 24110,
      logLevel: 'debug',
      coreAutostart: true,
    })
    expect(settingsPayload(draft, current, true).coreAutostart).toBe(false)
  })

  it('preserves Connection-owned port when the settings surface cannot mutate it', () => {
    const current = { port: 8767, logLevel: 'info', coreAutostart: true }
    const draft = { port: 19000, logLevel: 'debug', coreAutostart: true }

    expect(settingsPayload(draft, current, true, false)).toEqual({
      port: 8767,
      logLevel: 'debug',
      coreAutostart: true,
    })
  })
})
