import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { setLocale } from '../../i18n'
import NexusDockPanel from './NexusDockPanel.vue'

// The backend pairing API remains implemented, but the deferred product page
// must not expose a control that gives users the impression Nexus is ready.
describe('NexusDock deferred UI', () => {
  it('renders a Traditional Chinese read-only status view with unknown initial state', async () => {
    setLocale('zh-Hant')
    const html = await renderToString(createSSRApp(NexusDockPanel))
    expect(html).toContain('開發中')
    expect(html).toContain('僅提供唯讀狀態檢視')
    expect(html).toContain('未知')
    expect(html).not.toContain('未儲存')
    expect(html).not.toContain('配對裝置')
    expect(html).not.toContain('重新啟動 Next Core 以套用配對')
  })

  it('does not wire Pair or Reconcile mutations in the deferred page', () => {
    const source = readFileSync(new URL('./NexusDockPanel.vue', import.meta.url), 'utf8')
    for (const disabledFlow of ['NexusPairForm', 'desktopApi.nexusPair', 'nexusReconcile', 'canReconcile', 'finishMutation']) {
      expect(source).not.toContain(disabledFlow)
    }
    expect(source).toContain('onMounted(refresh)')
  })
})
