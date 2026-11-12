import { describe, expect, it } from 'vitest'
import type { PluginCandidateReview, PluginManagedItem } from '../api/desktopApi'
import { candidateDiff, candidateSummary, newPluginRequestID, pluginNeedsAttention, shortFingerprint } from './pluginLogic'

describe('pluginLogic', () => {
  it('creates exact 32-character lowercase hex request ids', () => {
    const source = {
      getRandomValues<T extends ArrayBufferView | null>(array: T): T {
        const bytes = array as Uint8Array
        bytes.forEach((_, index) => { bytes[index] = index })
        return array
      },
    } as Pick<Crypto, 'getRandomValues'>
    expect(newPluginRequestID(source)).toBe('000102030405060708090a0b0c0d0e0f')
  })

  it('marks recovery and warning states as attention', () => {
    const base = { warningCount: 0, recoveryState: '' } as PluginManagedItem
    expect(pluginNeedsAttention(base)).toBe(false)
    expect(pluginNeedsAttention({ ...base, warningCount: 1 })).toBe(true)
    expect(pluginNeedsAttention({ ...base, recoveryState: 'cleanup_required' })).toBe(true)
  })

  it('summarises candidate contents without exposing package data', () => {
    const review = {
      valid: true, name: 'demo', version: '1.0.0', description: '', format: 'portable', packageFingerprint: 'sha256:test',
      skills: [{ name: 'one', description: '' }],
      mcp: [{ name: 'remote', description: '', transport: 'streamable_http', environmentNames: [], headerNames: [] }],
      executables: ['bin/tool'],
      warnings: ['warn'],
      issues: [],
    } as PluginCandidateReview
    expect(candidateSummary(review)).toEqual({ skills: 1, mcp: 1, executables: 1, warnings: 1, issues: 0 })
    expect(shortFingerprint('sha256:abcdefghijklmnopqrstuvwxyz0123456789')).toContain('…')
  })

  it('diffs reviewed update capabilities by semantic name and safe fields', () => {
    const current = {
      valid: true, name: 'demo', version: '1.0.0', description: '', format: 'portable', packageFingerprint: 'sha256:old',
      skills: [{ name: 'same', description: 'old' }, { name: 'removed', description: '' }],
      mcp: [{ name: 'remote', description: '', transport: 'streamable_http', endpoint: 'https://old.example', environmentNames: [], headerNames: [] }],
      executables: ['bin/old'],
      warnings: ['old warning'],
      issues: [],
    } as PluginCandidateReview
    const next = {
      ...current,
      version: '2.0.0',
      packageFingerprint: 'sha256:new',
      skills: [{ name: 'same', description: 'new' }, { name: 'added', description: '' }],
      mcp: [{ name: 'remote', description: '', transport: 'streamable_http', endpoint: 'https://new.example', environmentNames: [], headerNames: [] }],
      executables: ['bin/new'],
      warnings: ['new warning'],
    } as PluginCandidateReview
    expect(candidateDiff(current, next)).toEqual({
      fingerprintChanged: true,
      skills: { added: ['added'], removed: ['removed'], changed: ['same'] },
      mcp: { added: [], removed: [], changed: ['remote'] },
      executables: { added: ['bin/new'], removed: ['bin/old'] },
      warnings: { added: ['new warning'], resolved: ['old warning'] },
    })
  })
})
