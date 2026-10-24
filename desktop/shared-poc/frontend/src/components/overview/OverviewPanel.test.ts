import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { setLocale } from '../../i18n'

const mocks = vi.hoisted(() => ({
  runtime: { stateKind: 'healthy', stateLabel: 'Healthy', loading: false, error: null, refresh: vi.fn() },
  connection: {
    snapshot: null as null | { coreRunning: boolean; publicMCPURL: string },
    busy: false,
    error: null as null | { code: string; category: string; message: string; retryable: boolean },
    operationError: null as null | { code: string; category: string; message: string; retryable: boolean },
    refresh: vi.fn(),
  },
  update: { channelDisabled: true, data: null, busy: false, error: null, refresh: vi.fn() },
  diagnostics: { data: null, busy: false, error: null, refresh: vi.fn() },
}))
vi.mock('../../stores/runtime', () => ({ useRuntimeStore: () => mocks.runtime }))
vi.mock('../../stores/connection', () => ({ useConnectionStore: () => mocks.connection }))
vi.mock('../../stores/update', () => ({ useUpdateStore: () => mocks.update }))
vi.mock('../../stores/diagnostics', () => ({ useDiagnosticsStore: () => mocks.diagnostics }))

import OverviewPanel from './OverviewPanel.vue'

beforeEach(() => {
  setLocale('zh-Hant')
  mocks.connection.snapshot = { coreRunning: true, publicMCPURL: 'https://next.example.test/mcp' }
  mocks.connection.busy = false
  mocks.connection.error = null
  mocks.connection.operationError = null
})

describe('Overview connection mutation feedback', () => {
  it('shows the rejected action even after the authoritative connection read succeeds', async () => {
    mocks.connection.operationError = {
      code: 'connection_action_rejected', category: 'operation', retryable: true, message: '',
    }
    const html = await renderToString(createSSRApp(OverviewPanel))
    expect(html).toContain('connection_action_rejected')
    expect(html).toContain('https://next.example.test/mcp')
  })

  it('still shows a snapshot read error when no action failure exists', async () => {
    mocks.connection.snapshot = null
    mocks.connection.error = {
      code: 'connection_snapshot_failed', category: 'unavailable', retryable: true, message: '',
    }
    mocks.connection.operationError = mocks.connection.error
    const html = await renderToString(createSSRApp(OverviewPanel))
    expect(html).toContain('connection_snapshot_failed')
    expect(html).not.toContain('https://next.example.test/mcp')
  })
})
