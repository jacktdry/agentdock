import { describe, expect, it } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import type { Snapshot } from '../../../bindings/github.com/uvwt/agentdock/internal/browserdesktop/models'
import { setLocale, t } from '../../i18n'
import BrowserReadinessGuidance from './BrowserReadinessGuidance.vue'
const snapshot = (): Snapshot => ({
  managedPoolMode: 'exclusive',
  observedAt: '2026-10-08T00:00:00Z', availability: 'available', state: 'leases_present', stale: false,
  browserEnabled: true, acpEnabled: true, companyRequiredEdgePolicies: 1,
  configuredConnectors: 2, configuredAuthenticatedEdgeProfiles: 3, configuredRequiredExternalPolicies: 4,
  connectorHealth: 'not_observed', managedLeases: 5, requiredExternalLeases: 6, explicitExternalLeases: 7,
  authenticatedTransportOwners: 0, owners: 1, leases: 18, workers: 1, activeLeases: 1, expiredLeases: 0, releasingLeases: 0,
  failedLeases: 0, unownedLeases: 0, readyWorkers: 1, failedWorkers: 0, activeOperations: 0,
  queuedOperations: 0, maxConcurrency: 4, queueCapacity: 8, managedOrphans: 0, externalOrphans: 0, lifecycleError: false,
})
const render = (s: Snapshot | null, error: string | null = null) =>
  renderToString(createSSRApp(BrowserReadinessGuidance, { snapshot: s, error }))
const healthy = (): Snapshot => ({ ...snapshot(), companyRequiredEdgePolicies: 0 })
describe('operator next-step guidance', () => {
  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('renders localized, accessible, passive guidance in %s', async locale => {
    setLocale(locale)
    for (const [s, key] of [
      [null, 'core'],
      [{ ...healthy(), availability: 'browser_disabled' }, 'browser'],
      [{ ...healthy(), availability: 'acp_disabled' }, 'acp'],
      [{ ...healthy(), availability: 'unknown' }, 'broker'],
      [{ ...snapshot(), stale: true }, 'cleanup'],
      [snapshot(), 'company'],
      [healthy(), 'session'],
    ] as const) {
      const html = await render(s)
      expect(t(`browser.next_${key}`)).not.toBe(`browser.next_${key}`)
      expect(html).toContain(t(`browser.next_${key}`).replaceAll("'", '&#39;'))
      expect(html).toContain('aria-labelledby="browser-next-title"')
      expect(html).toContain('aria-live="polite"')
      expect(html).not.toMatch(/<(button|input|a|dl|ul|table)[ >]/)
      expect(html).not.toContain('<dd>')
    }
  })
  it('prioritizes failed Core over retained counts and disabled settings over anomalies', async () => {
    setLocale('en')
    expect(await render({ ...snapshot(), stale: true, failedLeases: 987654 }, 'browser.snapshot_unavailable')).toContain(t('browser.next_core'))
    expect(await render(null)).not.toContain('987654')
    for (const [availability, key] of [['core_unavailable', 'core'], ['browser_disabled', 'browser'], ['acp_disabled', 'acp'], ['broker_unavailable', 'broker']] as const) {
      const html = await render({ ...snapshot(), availability, failedLeases: 987654 })
      expect(html).toContain(t(`browser.next_${key}`).replaceAll("'", '&#39;'))
      expect(html).not.toContain('987654')
      expect(html).not.toContain(t('browser.next_session'))
    }
  })
  it('prioritizes cleanup over company configuration and session admission', async () => {
    setLocale('en')
    const anomalies: Partial<Snapshot>[] = [
      { stale: true }, { state: 'stale' }, { lifecycleError: true },
      ...['expiredLeases', 'releasingLeases', 'failedLeases', 'unownedLeases', 'failedWorkers', 'managedOrphans', 'externalOrphans'].map(key => ({ [key]: 1 })),
      { managedPoolMode: 'unknown' },
      { managedPoolMode: 'experimental_shared' },
      { managedPoolMode: 'experimental_shared', sharedPoolBudget: { capacity: 2, occupied: 1, available: 1, claimed: 1, uncertain: 1 } },
    ]
    for (const anomaly of anomalies) {
      const html = await render({ ...snapshot(), ...anomaly })
      expect(html).toContain(t('browser.next_cleanup'))
      expect(html).not.toContain(t('browser.next_company'))
      expect(html).not.toContain(t('browser.next_session'))
    }
  })
  it('never invents verified connector health from an unknown health string or configured count', async () => {
    setLocale('en')
    expect(await render({ ...snapshot(), connectorHealth: 'healthy' })).toContain(t('browser.next_company'))
    expect(await render({ ...healthy(), state: 'unknown' })).not.toContain(t('browser.next_session'))
    for (const state of ['idle', 'leases_present']) {
      const html = await render({ ...healthy(), state, configuredAuthenticatedEdgeProfiles: 987654 })
      expect(html).toContain(t('browser.next_session'))
      expect(html).toContain(t('browser.next_exclusive'))
      expect(html).not.toContain('987654')
      expect(html).not.toMatch(/(Chrome|Edge) (is attached|is authenticated|is ready)/)
      expect(html).toContain('does not prove Chrome/Edge ready or consent')
    }
    const html = await render({ ...healthy(), managedPoolMode: 'experimental_shared', sharedPoolBudget: { capacity: 2, occupied: 0, available: 2, claimed: 0, uncertain: 0 } })
    expect(html).toContain(t('browser.next_session'))
    expect(html).toContain(t('browser.next_shared'))
    expect(html).not.toContain(t('browser.next_exclusive'))
  })
  it.each(['en', 'zh-Hant', 'zh-Hans'] as const)('does not infer admission from unknown counts in %s', async locale => {
    setLocale(locale)
    for (const count of [NaN, -1, 0.5, Infinity]) {
      for (const field of ['companyRequiredEdgePolicies', 'failedLeases', 'owners', 'configuredConnectors'] as const) {
        const html = await render({ ...healthy(), [field]: count })
        expect(html).toContain(t('browser.next_cleanup'))
        expect(html).not.toContain(t('browser.next_session'))
      }
    }
  })
})
