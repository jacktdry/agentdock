<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { MCPConfigInput, MCPManagedServer } from '../../api/desktopApi'
import { formatBindingLines, parseBindingLines, validateMCPConfig, type MCPValidationErrors } from '../../features/mcpLogic'
import { useI18n, type MessageKey } from '../../i18n'

const props = defineProps<{
  server?: MCPManagedServer | null
  registryRevision: string
  busy: boolean
}>()
const emit = defineEmits<{
  save: [input: MCPConfigInput, revision: string, generation: string]
  cancel: []
}>()
const { t } = useI18n()

const name = ref('')
const description = ref('')
const transport = ref('streamable_http')
const url = ref('')
const command = ref('')
const protocolVersion = ref('')
const timeoutMs = ref(30000)
const cwd = ref('')
const argsText = ref('')
const headerEnvText = ref('')
const envFromEnvText = ref('')
const replaceProtectedURL = ref(false)
const replaceProtectedArgs = ref(false)
const errors = ref<MCPValidationErrors>({})

const editing = computed(() => !!props.server)
const title = computed(() => t(editing.value ? 'mcp.edit_server' : 'mcp.add_server'))

function reset() {
  const server = props.server
  name.value = server?.name ?? ''
  description.value = server?.description ?? ''
  transport.value = server?.transport ?? 'streamable_http'
  url.value = server?.urlProtected ? '' : (server?.url ?? '')
  command.value = server?.command ?? ''
  protocolVersion.value = server?.protocolVersion ?? ''
  timeoutMs.value = server?.timeoutMs ?? 30000
  cwd.value = server?.cwd ?? ''
  argsText.value = server?.argsProtected ? '' : (server?.args ?? []).join('\n')
  headerEnvText.value = formatBindingLines(server?.headerEnv)
  envFromEnvText.value = formatBindingLines(server?.envFromEnv)
  replaceProtectedURL.value = false
  replaceProtectedArgs.value = false
  errors.value = {}
}

function submit() {
  const input: MCPConfigInput = {
    name: name.value.trim(),
    description: description.value.trim(),
    transport: transport.value,
    protocolVersion: protocolVersion.value || undefined,
    timeoutMs: timeoutMs.value,
    headerEnv: parseBindingLines(headerEnvText.value),
    envFromEnv: parseBindingLines(envFromEnvText.value),
  }

  if (transport.value === 'streamable_http') {
    if (!props.server?.urlProtected || replaceProtectedURL.value) input.url = url.value.trim()
  } else {
    input.command = command.value.trim()
    input.cwd = cwd.value.trim() || undefined
    if (!props.server?.argsProtected || replaceProtectedArgs.value) {
      input.args = argsText.value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)
    }
  }

  errors.value = validateMCPConfig(input, !editing.value)
  if (Object.keys(errors.value).length > 0) return
  emit('save', input, props.registryRevision, props.server?.generation ?? '')
}

const fieldError = (key: keyof MCPValidationErrors) => {
  const value = errors.value[key]
  return value ? t(value as MessageKey) : ''
}

watch(() => props.server, reset, { immediate: true })
onMounted(() => reset())
</script>

<template>
  <dialog open class="mcp-editor" aria-labelledby="mcp-editor-title" @cancel.prevent="emit('cancel')">
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ t('mcp.configuration') }}</p>
        <h2 id="mcp-editor-title">{{ title }}</h2>
      </div>
    </div>

    <form class="settings-form mcp-editor-form" @submit.prevent="submit">
      <label>
        {{ t('mcp.name') }}
        <input v-model="name" type="text" :disabled="editing || busy" :aria-invalid="!!errors.name" aria-describedby="mcp-name-hint mcp-name-error" />
      </label>
      <p id="mcp-name-hint" class="field-hint">{{ editing ? t('mcp.name_immutable') : t('mcp.name_hint') }}</p>
      <p v-if="errors.name" id="mcp-name-error" class="error">{{ fieldError('name') }}</p>

      <label>
        {{ t('mcp.description') }}
        <input v-model="description" type="text" :disabled="busy" :aria-invalid="!!errors.description" aria-describedby="mcp-description-error" />
      </label>
      <p v-if="errors.description" id="mcp-description-error" class="error">{{ fieldError('description') }}</p>

      <label>
        {{ t('mcp.transport') }}
        <select v-model="transport" :disabled="busy">
          <option value="streamable_http">{{ t('mcp.transport_http') }}</option>
          <option value="stdio">{{ t('mcp.transport_stdio') }}</option>
        </select>
      </label>

      <template v-if="transport === 'streamable_http'">
        <div v-if="server?.urlProtected && !replaceProtectedURL" class="protected-field">
          <strong>{{ t('mcp.endpoint_protected') }}</strong>
          <p class="field-hint">{{ t('mcp.endpoint_protected_hint') }}</p>
          <button type="button" :disabled="busy" @click="replaceProtectedURL = true">{{ t('mcp.replace_endpoint') }}</button>
        </div>
        <label v-else>
          {{ t('mcp.endpoint') }}
          <input v-model="url" type="url" :disabled="busy" :aria-invalid="!!errors.url" aria-describedby="mcp-url-hint mcp-url-error" />
        </label>
        <p id="mcp-url-hint" class="field-hint">{{ t('mcp.endpoint_hint') }}</p>
        <p v-if="errors.url" id="mcp-url-error" class="error">{{ fieldError('url') }}</p>
      </template>

      <template v-else>
        <label>
          {{ t('mcp.command') }}
          <input v-model="command" type="text" class="path-input" :disabled="busy" :aria-invalid="!!errors.command" aria-describedby="mcp-command-error" />
        </label>
        <p v-if="errors.command" id="mcp-command-error" class="error">{{ fieldError('command') }}</p>
        <p class="field-hint">{{ t('mcp.stdio_warning') }}</p>
      </template>

      <details>
        <summary>{{ t('mcp.advanced') }}</summary>
        <div class="mcp-editor-advanced">
          <label>
            {{ t('mcp.protocol_version') }}
            <select v-model="protocolVersion" :disabled="busy">
              <option value="">{{ t('mcp.protocol_auto') }}</option>
              <option value="2024-11-05">2024-11-05</option>
              <option value="2025-03-26">2025-03-26</option>
              <option value="2025-06-18">2025-06-18</option>
              <option value="2025-11-25">2025-11-25</option>
              <option value="2026-07-28">2026-07-28</option>
            </select>
          </label>
          <label>
            {{ t('mcp.timeout') }}
            <input v-model.number="timeoutMs" type="number" min="1" max="300000" :disabled="busy" :aria-invalid="!!errors.timeoutMs" />
          </label>
          <p v-if="errors.timeoutMs" class="error">{{ fieldError('timeoutMs') }}</p>

          <template v-if="transport === 'streamable_http'">
            <label>
              {{ t('mcp.header_env') }}
              <textarea v-model="headerEnvText" rows="4" :disabled="busy" placeholder="Authorization=REMOTE_MCP_AUTH" />
            </label>
            <p class="field-hint">{{ t('mcp.header_env_hint') }}</p>
          </template>

          <template v-else>
            <label>
              {{ t('mcp.cwd') }}
              <input v-model="cwd" type="text" class="path-input" :disabled="busy" :aria-invalid="!!errors.cwd" />
            </label>
            <p v-if="errors.cwd" class="error">{{ fieldError('cwd') }}</p>

            <div v-if="server?.argsProtected && !replaceProtectedArgs" class="protected-field">
              <strong>{{ t('mcp.args_protected') }}</strong>
              <p class="field-hint">{{ t('mcp.args_protected_hint') }}</p>
              <button type="button" :disabled="busy" @click="replaceProtectedArgs = true">{{ t('mcp.replace_args') }}</button>
            </div>
            <label v-else>
              {{ t('mcp.args') }}
              <textarea v-model="argsText" rows="5" :disabled="busy" :aria-invalid="!!errors.args" />
            </label>
            <p v-if="errors.args" class="error">{{ fieldError('args') }}</p>

            <label>
              {{ t('mcp.child_env') }}
              <textarea v-model="envFromEnvText" rows="4" :disabled="busy" placeholder="CHILD_TOKEN=HOST_TOKEN" />
            </label>
            <p class="field-hint">{{ t('mcp.child_env_hint') }}</p>
          </template>
        </div>
      </details>

      <p class="field-hint">{{ t('mcp.create_disabled_hint') }}</p>
      <div class="actions">
        <button type="button" autofocus :disabled="busy" @click="emit('cancel')">{{ t('common.cancel') }}</button>
        <button type="submit" :disabled="busy">{{ t('common.save') }}</button>
      </div>
    </form>
  </dialog>
</template>
