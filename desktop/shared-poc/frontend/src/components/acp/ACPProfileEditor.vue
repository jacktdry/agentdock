<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import type { ACPManagedProfile } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopapi/models'
import type { ACPProfileSettings } from '../../../bindings/github.com/uvwt/agentdock/internal/desktopruntime/models'
import { useI18n } from '../../i18n'
import {
  moveACPArgument,
  profileRuntimeKind,
  type ACPProfileEditorPreset,
} from '../../features/acpProfileEditorLogic'

const props = defineProps<{
  profile?: ACPManagedProfile | null
  busy: boolean
  existingIds: string[]
}>()
const emit = defineEmits<{
  save: [profile: ACPProfileSettings, originalId?: string]
  cancel: []
}>()

const { t } = useI18n()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const preset = ref<ACPProfileEditorPreset>('codex')
const id = ref('')
const displayName = ref('')
const command = ref('')
const args = ref<string[]>([])
const enabled = ref(false)
const validation = ref('')

const editing = computed(() => !!props.profile)
const originalId = computed(() => props.profile?.id)
const customIdVisible = computed(() => preset.value === 'custom')
const argsProtected = computed(() => props.profile?.protectedArgs === true)

function inferPreset(profile: ACPManagedProfile): ACPProfileEditorPreset {
  if (profile.preset === 'codex') return 'codex'
  if (profile.preset === 'antigravity') return 'antigravity'
  if (profile.preset === 'legacy') return 'legacy'
  return 'custom'
}

function resetDraft() {
  const profile = props.profile
  if (profile) {
    preset.value = inferPreset(profile)
    id.value = profile.id
    displayName.value = profile.displayName ?? ''
    command.value = profile.configuredCommand ?? ''
    args.value = [...(profile.configuredArgs ?? [])]
    enabled.value = profile.enabled
  } else {
    preset.value = 'codex'
    id.value = 'codex'
    displayName.value = t('acp.preset_codex')
    command.value = ''
    args.value = []
    enabled.value = false
  }
  validation.value = ''
}

watch(() => props.profile, resetDraft, { immediate: true })
onMounted(() => dialog.value?.showModal())

watch(preset, value => {
  if (editing.value) return
  if (value === 'codex') {
    id.value = 'codex'
    displayName.value = t('acp.preset_codex')
  } else if (value === 'antigravity') {
    id.value = 'antigravity'
    displayName.value = t('acp.preset_antigravity')
  } else {
    id.value = ''
    displayName.value = ''
  }
})

function addArgument() {
  args.value = [...args.value, '']
}

function removeArgument(index: number) {
  args.value = args.value.filter((_, current) => current !== index)
}

function moveArgument(index: number, direction: -1 | 1) {
  args.value = moveACPArgument(args.value, index, direction)
}

function submit() {
  validation.value = ''
  const trimmedId = id.value.trim()
  const trimmedCommand = command.value.trim()
  if (customIdVisible.value && !trimmedId) {
    validation.value = t('acp.validation_id')
    return
  }
  if (!editing.value && trimmedId && props.existingIds.includes(trimmedId)) {
    validation.value = t('acp.validation_duplicate')
    return
  }
  if (enabled.value && !trimmedCommand) {
    validation.value = t('acp.validation_command')
    return
  }

  const kind = profileRuntimeKind(props.profile?.runtimeKind, preset.value)
  const profile: ACPProfileSettings = {
    id: trimmedId,
    displayName: displayName.value.trim(),
    kind,
    command: trimmedCommand,
    args: argsProtected.value ? [] : [...args.value],
    preserveArgs: argsProtected.value,
    enabled: enabled.value,
  }
  emit('save', profile, originalId.value)
}
</script>

<template>
  <dialog ref="dialog" class="profile-editor" aria-labelledby="acp-profile-editor-title" @cancel.prevent="emit('cancel')">
    <form class="settings-form" @submit.prevent="submit">
      <h2 id="acp-profile-editor-title">{{ editing ? t('acp.edit_profile') : t('acp.add_profile') }}</h2>
      <fieldset :disabled="busy">
        <label>
          {{ t('acp.adapter_type') }}
          <select v-model="preset" :disabled="editing">
            <option value="codex">{{ t('acp.preset_codex') }}</option>
            <option value="antigravity">{{ t('acp.preset_antigravity') }}</option>
            <option value="custom">{{ t('acp.preset_custom') }}</option>
            <option v-if="editing && preset === 'legacy'" value="legacy">{{ t('acp.preset_legacy') }}</option>
          </select>
        </label>
        <p class="field-hint">{{ preset === 'legacy' ? t('acp.preset_help_legacy') : t(('acp.preset_help_' + preset) as any) }}</p>

        <label v-if="customIdVisible">
          {{ t('acp.profile_id') }}
          <input v-model.trim="id" type="text" maxlength="64" :disabled="editing" autocomplete="off">
        </label>

        <label>
          {{ t('acp.display_name') }}
          <input v-model="displayName" type="text" maxlength="80" autocomplete="off">
        </label>

        <label class="checkbox-label">
          <input v-model="enabled" type="checkbox">
          {{ t('common.enabled') }}
        </label>
        <p class="field-hint">{{ t('acp.enabled_command_hint') }}</p>

        <details>
          <summary>{{ t('acp.advanced') }}</summary>
          <div class="profile-editor-advanced">
            <label>
              {{ t('acp.command') }}
              <input v-model="command" type="text" spellcheck="false" autocomplete="off" class="path-input">
            </label>
            <p class="field-hint">{{ t('acp.command_hint') }}</p>

            <div class="argument-editor">
              <div class="argument-editor-heading">
                <span>{{ t('acp.arguments') }}</span>
                <button v-if="!argsProtected" type="button" :disabled="busy" @click="addArgument">{{ t('acp.add_argument') }}</button>
              </div>

              <p v-if="argsProtected" class="field-hint">{{ t('acp.protected_arguments_hint') }}</p>
              <p v-else-if="args.length === 0" class="field-hint">{{ t('acp.arguments_empty') }}</p>

              <div v-for="(_, index) in args" :key="index" class="argument-row">
                <textarea v-model="args[index]" rows="2" spellcheck="false"
                  :aria-label="t('acp.argument_named', { index: index + 1 })" />
                <div class="argument-actions">
                  <button type="button" :disabled="busy || index === 0"
                    :aria-label="t('acp.move_argument_up', { index: index + 1 })"
                    @click="moveArgument(index, -1)">↑</button>
                  <button type="button" :disabled="busy || index === args.length - 1"
                    :aria-label="t('acp.move_argument_down', { index: index + 1 })"
                    @click="moveArgument(index, 1)">↓</button>
                  <button type="button" class="danger-button" :disabled="busy"
                    :aria-label="t('acp.remove_argument', { index: index + 1 })"
                    @click="removeArgument(index)">×</button>
                </div>
              </div>
            </div>
          </div>
        </details>
      </fieldset>

      <p v-if="validation" class="error" role="alert">{{ validation }}</p>
      <div class="actions">
        <button type="button" autofocus :disabled="busy" @click="emit('cancel')">{{ t('common.cancel') }}</button>
        <button type="submit" :disabled="busy">{{ t('common.save') }}</button>
      </div>
    </form>
  </dialog>
</template>
