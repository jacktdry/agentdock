export type ACPProfileEditorPreset = 'codex' | 'antigravity' | 'custom' | 'legacy'

export function profileRuntimeKind(existingRuntimeKind: string | undefined, preset: ACPProfileEditorPreset): string {
  if (existingRuntimeKind) return existingRuntimeKind
  return preset === 'codex' ? 'codex' : 'custom'
}

export function moveACPArgument(args: string[], index: number, direction: -1 | 1): string[] {
  const target = index + direction
  if (index < 0 || index >= args.length || target < 0 || target >= args.length) return [...args]
  const next = [...args]
  const [value] = next.splice(index, 1)
  next.splice(target, 0, value)
  return next
}
