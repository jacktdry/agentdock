import { describe, expect, it } from 'vitest'
import { moveACPArgument, profileRuntimeKind } from './acpProfileEditorLogic'

describe('ACP profile editor logic', () => {
  it('preserves an existing legacy runtime kind while editing', () => {
    expect(profileRuntimeKind('claude', 'legacy')).toBe('claude')
    expect(profileRuntimeKind('grok', 'legacy')).toBe('grok')
    expect(profileRuntimeKind(undefined, 'codex')).toBe('codex')
    expect(profileRuntimeKind(undefined, 'antigravity')).toBe('custom')
  })

  it('reorders argument values without normalizing their contents', () => {
    const args = ['  value  ', '', 'line1\nline2', '--flag=value']
    expect(moveACPArgument(args, 2, -1)).toEqual(['  value  ', 'line1\nline2', '', '--flag=value'])
    expect(moveACPArgument(args, 0, -1)).toEqual(args)
    expect(moveACPArgument(args, 3, 1)).toEqual(args)
  })
})
