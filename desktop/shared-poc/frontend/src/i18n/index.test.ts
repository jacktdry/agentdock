import { beforeEach, describe, expect, it } from 'vitest'
import { currentLocale, currentPreference, setLocale, t } from './index'

describe('shared UI i18n', () => {
  beforeEach(() => {
    setLocale('en')
  })

  it('formats ICU arguments from the canonical catalog', () => {
    expect(t('activity.summary_active', { delivered: '12', dropped: '3' })).toBe(
      'Activity stream active. 12 delivered, 3 dropped.',
    )
  })

  it('switches to Traditional Chinese without changing keys', () => {
    setLocale('zh-Hant')
    expect(currentLocale.value).toBe('zh-Hant')
    expect(t('common.refresh')).toBe('重新整理')
    expect(t('runtime.status_summary', { state: '正常' })).toBe('執行階段狀態：正常。')
  })

  it('switches to Simplified Chinese', () => {
    setLocale('zh-Hans')
    expect(t('common.refresh')).toBe('刷新')
  })

  it('can return to automatic system-language mode', () => {
    setLocale('zh-Hant')
    setLocale('system')
    expect(currentPreference.value).toBe('system')
    expect(t('common.follow_system')).not.toBe('')
  })
})
